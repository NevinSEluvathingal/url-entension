package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"strconv"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"

)


const dbFile = "comments.db"

var neo4jDriver neo4j.Driver


type User struct {
	ID       int    `json:"user_id"`
	Username string `json:"username"`
}

// Comment struct
type Comment struct {
	ID             int    `json:"id"`
	UserID         int    `json:"user_id"`
	ParentID       *int   `json:"parent_id,omitempty"`
	Comment        string `json:"comment"`
	CreatedAt      string `json:"created_at"`
	Username       string `json:"username"`
	ProfilePic     string `json:"profile_pic"`
	SentimentScore int    `json:"sentiment_score"`
	ReplyCount     int    `json:"reply_count"`
	LikeCount      int    `json:"like_count"`
	DislikeCount   int    `json:"dislike_count"`
	LikeStatus     *bool   `json:"like_status"`
	ConStatus      *bool   `json:"con_status"`
	URL            string `json:"url,omitempty"`
}


var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}

	
	createTables()
	
		
	neo4jDriver, err = neo4j.NewDriver("neo4j://localhost:7687", neo4j.BasicAuth("neo4j", "nevin1704%", ""))
	if err != nil {
		log.Fatal("Error connecting to Neo4j:", err)
	}
}

func createTables() {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY,
		username TEXT NOT NULL UNIQUE
	);

	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		parent_id INTEGER,
		user_id INTEGER NOT NULL,
		username TEXT NOT NULL,
		profile_pic TEXT,
		comment TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		sentiment_score INTEGER DEFAULT 0,
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (parent_id) REFERENCES comments(id)
	);

	CREATE TABLE IF NOT EXISTS comment_likes (
		id INTEGER PRIMARY KEY,
		comment_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		is_like BOOLEAN NOT NULL,
		FOREIGN KEY (comment_id) REFERENCES comments(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	
	CREATE TABLE IF NOT EXISTS connection (
		user_id INTEGER NOT NULL,
		comment_id INTEGER NOT NULL
	)	
	`

	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}
}

func main() {
	r := mux.NewRouter()

	
	r.Use(enableCORS)

	// Define Routes
	r.HandleFunc("/login/{username}", login).Methods("GET")
	r.HandleFunc("/comments", getComments).Methods("GET")
	r.HandleFunc("/replie/{comment_id}", getReplies).Methods("GET")
	r.HandleFunc("/comments", postComment).Methods("POST")
	r.HandleFunc("/replies/{parent_id}", postReply).Methods("POST")
	r.HandleFunc("/comment_like", commentLike).Methods("POST") 
	r.HandleFunc("/connect_users", connectUsers).Methods("POST")
	r.HandleFunc("/comments_by_connections", getCommentsByConnections).Methods("GET")
	r.HandleFunc("/disconnect_users", disconnectUsers).Methods("DELETE")
	
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// CORS Middleware to allow cross-origin requests
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("CORS middleware triggered") 

		
		w.Header().Set("Access-Control-Allow-Origin", "*")  
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS") 
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization") 

		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		
		next.ServeHTTP(w, r)
	})
}

func connectUsers(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserID1 int `json:"user_id_1"`
		UserID2 int `json:"user_id_2"`
		ComID   int `json:"com_id"`
	}
	
	

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Println(request)
	
	query := `INSERT INTO connection (user_id, comment_id) VALUES (?, ?)`
        _, err := db.Exec(query, request.UserID1, request.ComID)
        if err != nil {
            http.Error(w, "Error inserting comment table", http.StatusInternalServerError)
            log.Println(err)
            return
        }

	session := neo4jDriver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	_, err = session.Run(
		"MERGE (u1:User {id: $userID1}) MERGE (u2:User {id: $userID2}) MERGE (u1)-[:CONNECTED]->(u2)",
		map[string]interface{}{
			"userID1": request.UserID1,
			"userID2": request.UserID2,
		},
	)
	if err != nil {
		http.Error(w, "Failed to connect users", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "Users connected"})
}

func disconnectUsers(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserID1 int `json:"user_id_1"`
		UserID2 int `json:"user_id_2"`
		ComID   int `json:"com_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Println("Disconnecting:", request)

	// Remove from SQL database
	query := `DELETE FROM connection WHERE user_id = ? AND comment_id = ?`
	_, err := db.Exec(query, request.UserID1, request.ComID)
	if err != nil {
		http.Error(w, "Error deleting from connection table", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	// Remove from Neo4j graph
	session := neo4jDriver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	_, err = session.Run(
		`MATCH (u1:User {id: $userID1})-[r:CONNECTED]-(u2:User {id: $userID2}) DELETE r`,
		map[string]interface{}{
			"userID1": request.UserID1,
			"userID2": request.UserID2,
		},
	)
	if err != nil {
		http.Error(w, "Failed to disconnect users", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "Users disconnected"})
}


func getCommentsByConnections(w http.ResponseWriter, r *http.Request) {
	urlParam := r.URL.Query().Get("url")
	IDparam := r.URL.Query().Get("user_id")

	decodedURL, err := url.QueryUnescape(urlParam)
	userID, err := strconv.Atoi(IDparam)
	if err != nil {
		http.Error(w, "Error decoding user ID", http.StatusBadRequest)
		log.Println(err)
		return
	}

	session := neo4jDriver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close()

	query := `
		MATCH (u:User {id: $userID})-[:CONNECTED*1..3]-(connectedUser)
		RETURN DISTINCT connectedUser.id AS userID
	`

	result, err := session.Run(query, map[string]interface{}{"userID": userID})
	if err != nil {
		http.Error(w, "Failed to fetch connected users", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	var userIDs []int
	for result.Next() {
		if id, ok := result.Record().Get("userID"); ok {
			log.Println("Connected user IDs:", id)
			if intID, ok := id.(int64); ok {
				userIDs = append(userIDs, int(intID))
			}
		}
	}

	if len(userIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	// Build SQL query with placeholders for user IDs
	queryStr := `
		SELECT c.id, c.user_id, c.parent_id, c.username, c.profile_pic, c.comment, c.created_at, c.sentiment_score,
		       (SELECT COUNT(*) FROM comments r WHERE r.parent_id = c.id) AS reply_count,
		       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
		       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count,
		       (SELECT is_like FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.user_id = ?) AS like_status,
		       EXISTS (
		       		SELECT 1 FROM connection WHERE user_id = ? AND comment_id = c.id
		       ) AS row_exists
		FROM comments c WHERE c.user_id IN (`

	args := []interface{}{userID, userID} // Pass userID for the first two `?` placeholders

	for i, id := range userIDs {
		if i > 0 {
			queryStr += ","
		}
		queryStr += "?"
		args = append(args, id) // Append user IDs to match `?` placeholders
	}

	queryStr += ") AND url = ?" // Add URL filtering
	args = append(args, decodedURL) // Append the URL to the query parameters

	rows, err := db.Query(queryStr, args...)
	if err != nil {
		http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.ParentID, &c.Username, &c.ProfilePic,
			&c.Comment, &c.CreatedAt, &c.SentimentScore, &c.ReplyCount,
			&c.LikeCount, &c.DislikeCount, &c.LikeStatus, &c.ConStatus,
		); err != nil {
			log.Println("Error scanning comment:", err)
			continue
		}
		comments = append(comments, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// Login route to create or fetch a user
func login(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	log.Printf("Received login request for username: %s", username)

	
	var userID int
	err := db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&userID)

	
	if err == sql.ErrNoRows {
		
		userID = generateRandomUserID()

		
		_, err := db.Exec("INSERT INTO users (id, username) VALUES (?, ?)", userID, username)
		if err != nil {
			http.Error(w, "Error creating user", http.StatusInternalServerError)
			log.Println(err)
			return
		}
	}

	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"user_id": userID})
}

// Generate a random 8-digit user ID
func generateRandomUserID() int {
	return 12345678 
}

// Get comments for a specific URL
func getComments(w http.ResponseWriter, r *http.Request) {
	
	urlParam := r.URL.Query().Get("url")
	idParam:=r.URL.Query().Get("user_id")
	
	
	decodedURL, err := url.QueryUnescape(urlParam)
	decodedID, err := url.QueryUnescape(idParam)
	if err != nil {
		http.Error(w, "Error decoding URL", http.StatusBadRequest)
		log.Println(err)
		return
	}

	log.Printf("Fetching comments for URL: %s", decodedURL)
	log.Printf("userID: %s", decodedID)
	

	query := `
	SELECT c.id, c.user_id, c.parent_id, c.comment, c.created_at, c.username, c.profile_pic, c.sentiment_score,
	       (SELECT COUNT(*) FROM comments r WHERE r.parent_id = c.id) AS reply_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count,
	       (SELECT is_like FROM comment_likes cl WHERE cl.comment_id=c.id AND cl.user_id=?) AS like_status,
	       (SELECT EXISTS(
    			SELECT 1 FROM connection WHERE user_id = ? AND comment_id = c.id
		)) AS row_exists
	       
	FROM comments c
	WHERE c.url = ? AND c.parent_id IS NULL
	ORDER BY c.created_at ASC;
	`

	rows, err := db.Query(query, decodedID, decodedID, decodedURL)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.ParentID, &c.Comment, &c.CreatedAt, &c.Username, &c.ProfilePic, &c.SentimentScore, &c.ReplyCount, &c.LikeCount, &c.DislikeCount, &c.LikeStatus,&c.ConStatus); err != nil {
			http.Error(w, "Error scanning results", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		comments = append(comments, c)
	}

	log.Printf("Comments retrieved: %+v", comments)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// Get replies for a specific comment
func getReplies(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	commentID := vars["comment_id"]
	log.Println(commentID)

	query := `SELECT * FROM comments WHERE parent_id = ?`
	rows, err := db.Query(query, commentID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer rows.Close()

	var replies []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.URL, &c.ParentID, &c.UserID, &c.Username, &c.ProfilePic, &c.Comment, &c.CreatedAt, &c.SentimentScore); err != nil {
			http.Error(w, "Error scanning results", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		replies = append(replies, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(replies)
}

// Post a comment
func postComment(w http.ResponseWriter, r *http.Request) {
    var c Comment

    
    if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Set the query for inserting the new comment
    query := `
        INSERT INTO comments (url, parent_id, user_id, username, profile_pic, comment, sentiment_score) 
        VALUES (?, ?, ?, ?, ?, ?, ?);
    `
    
    result, err := db.Exec(query, c.URL, c.ParentID, c.UserID, c.Username, c.ProfilePic, c.Comment, c.SentimentScore)
    if err != nil {
        http.Error(w, "Database insert error", http.StatusInternalServerError)
        log.Println(err)
        return
    }

    
    commentID, err := result.LastInsertId()
    if err != nil {
        http.Error(w, "Failed to retrieve comment ID", http.StatusInternalServerError)
        log.Println(err)
        return
    }

    
    c.ID = int(commentID)

    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"comment_id": c.ID})
}

// Post a reply
func postReply(w http.ResponseWriter, r *http.Request) {
    log.Println("triggered post")

    var c Comment
    vars := mux.Vars(r)
    parentID := vars["parent_id"]

    
    if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    log.Println("reached1")

    
    parentIDInt, err := strconv.Atoi(parentID)
    if err != nil {
        log.Println("Invalid parent ID:", parentID)
        http.Error(w, "Invalid parent ID", http.StatusBadRequest)
        return
    }

    
    query := `
        INSERT INTO comments (url, parent_id, user_id, username, profile_pic, comment, sentiment_score) 
        VALUES (?, ?, ?, ?, ?, ?, ?);
    `
    result, err := db.Exec(query, c.URL, parentIDInt, c.UserID, c.Username, c.ProfilePic, c.Comment, c.SentimentScore)
    if err != nil {
        log.Println("Error executing query:", err)
        http.Error(w, "Database insert error", http.StatusInternalServerError)
        return
    }

    
    commentID, err := result.LastInsertId()
    if err != nil {
        log.Println("Error retrieving last insert ID:", err)
        http.Error(w, "Failed to retrieve comment ID", http.StatusInternalServerError)
        return
    }

    c.ID = int(commentID) 
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]int{"comment_id": c.ID})
}

func commentLike(w http.ResponseWriter, r *http.Request) {
    var request struct {
        UserID    int  `json:"user_id"`
        CommentID int  `json:"comment_id"`
        IsLike    bool `json:"is_like"` 
    }

    
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        log.Println("invalid structure")
        return
    }

    
    var existingLikeID int
    var existingIsLike bool
    err := db.QueryRow("SELECT id, is_like FROM comment_likes WHERE comment_id = ? AND user_id = ?", request.CommentID, request.UserID).Scan(&existingLikeID, &existingIsLike)

    if err == sql.ErrNoRows {
        
        query := `INSERT INTO comment_likes (comment_id, user_id, is_like) VALUES (?, ?, ?)`
        _, err := db.Exec(query, request.CommentID, request.UserID, request.IsLike)
        if err != nil {
            http.Error(w, "Error inserting like/dislike", http.StatusInternalServerError)
            log.Println(err)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "1"}) 
    } else if err == nil {
        if existingIsLike && request.IsLike {
            query := `DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?`
            _, err := db.Exec(query, request.CommentID, request.UserID)
            if err != nil {
                http.Error(w, "Error removing like", http.StatusInternalServerError)
                log.Println(err)
                return
            }
           
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]string{"status": "2"})
            return
        }
        if !existingIsLike && !request.IsLike {
          
            query := `DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?`
            _, err := db.Exec(query, request.CommentID, request.UserID)
            if err != nil {
                http.Error(w, "Error removing like", http.StatusInternalServerError)
                log.Println(err)
                return
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]string{"status": "3"})
            return
        }
        if existingIsLike && !request.IsLike {
           
            query := `UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?`
            _, err := db.Exec(query, request.IsLike, request.CommentID, request.UserID)
            if err != nil {
                http.Error(w, "Error updating like", http.StatusInternalServerError)
                log.Println(err)
                return
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]string{"status": "4"})
            return
        }
        if !existingIsLike && request.IsLike {
           
            query := `UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?`
            _, err := db.Exec(query, request.IsLike, request.CommentID, request.UserID)
            if err != nil {
                http.Error(w, "Error updating like", http.StatusInternalServerError)
                log.Println(err)
                return
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(map[string]string{"status": "5"})
            return
        }
    } else {
        http.Error(w, "Database error", http.StatusInternalServerError)
        log.Println(err)
    }
}


