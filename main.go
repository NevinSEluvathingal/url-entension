package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

// Database file
const dbFile = "comments.db"

// Comment struct
type Comment struct {
	ID             int    `json:"id"`
	ParentID       *int   `json:"parent_id,omitempty"`
	Comment        string `json:"comment"`
	CreatedAt      string `json:"created_at"`
	Username       string `json:"username"`
	ProfilePic     string `json:"profile_pic"`
	SentimentScore int    `json:"sentiment_score"`
	ReplyCount     int    `json:"reply_count"`
	LikeCount      int    `json:"like_count"`
	DislikeCount   int    `json:"dislike_count"`
	URL            string `json:"url,omitempty"`
}

// Database connection
var db *sql.DB

func init() {
	var err error
	db, err = sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}

	// Create tables if they don't exist
	createTables()
}

func createTables() {
	query := `
	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY,
		url TEXT NOT NULL,
		parent_id INTEGER,
		username TEXT NOT NULL,
		profile_pic TEXT,
		comment TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		sentiment_score INTEGER DEFAULT 0,
		FOREIGN KEY (parent_id) REFERENCES comments(id)
	);

	CREATE TABLE IF NOT EXISTS comment_likes (
		id INTEGER PRIMARY KEY,
		comment_id INTEGER NOT NULL,
		is_like BOOLEAN NOT NULL,
		FOREIGN KEY (comment_id) REFERENCES comments(id)
	);
	`

	_, err := db.Exec(query)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}
}

func main() {
	r := mux.NewRouter()

	// Apply CORS middleware to all routes
	r.Use(corsMiddleware)

	// Define Routes
	r.HandleFunc("/comments", getComments).Methods("GET")
	r.HandleFunc("/replies/{comment_id}", getReplies).Methods("GET")
	r.HandleFunc("/comments", postComment).Methods("POST")

	// Start Server
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// CORS Middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins, you can restrict this to specific domains
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// If the method is OPTIONS, return a 200 status
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass the request to the next handler
		next.ServeHTTP(w, r)
	})
}

// Get main comments for a URL
func getComments(w http.ResponseWriter, r *http.Request) {
	// Decode URL parameter
	urlParam := r.URL.Query().Get("url")
	// Decode the URL to handle any encoded characters
	decodedURL, err := url.QueryUnescape(urlParam)
	if err != nil {
		http.Error(w, "Error decoding URL", http.StatusBadRequest)
		log.Println(err)
		return
	}

	log.Printf("Fetching comments for URL: %s", decodedURL)

	query := `
	SELECT c.id, c.parent_id, c.comment, c.created_at, c.username, c.profile_pic, c.sentiment_score,
	       (SELECT COUNT(*) FROM comments r WHERE r.parent_id = c.id) AS reply_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count
	FROM comments c
	WHERE c.url = ? AND c.parent_id IS NULL
	ORDER BY c.created_at ASC;
	`

	rows, err := db.Query(query, decodedURL)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Comment, &c.CreatedAt, &c.Username, &c.ProfilePic, &c.SentimentScore, &c.ReplyCount, &c.LikeCount, &c.DislikeCount); err != nil {
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

	query := `
	SELECT c.id, c.parent_id, c.comment, c.created_at, c.username, c.profile_pic, c.sentiment_score,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count
	FROM comments c
	WHERE c.parent_id = ?
	ORDER BY c.created_at ASC;
	`

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
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Comment, &c.CreatedAt, &c.Username, &c.ProfilePic, &c.SentimentScore, &c.LikeCount, &c.DislikeCount); err != nil {
			http.Error(w, "Error scanning results", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		replies = append(replies, c)
	}

	log.Printf("Replies retrieved: %+v", replies)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(replies)
}

// Post a new comment with a provided ID from the frontend
func postComment(w http.ResponseWriter, r *http.Request) {
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	log.Printf("Received comment: %+v", c)

	// Ensure that the required fields are provided
	if c.URL == "" || c.Comment == "" || c.Username == "" || c.ID == 0 {
		http.Error(w, "Missing required fields (including ID)", http.StatusBadRequest)
		return
	}
	query := `
	INSERT INTO comments (id, url, parent_id, username, profile_pic, comment, sentiment_score) 
	VALUES (?, ?, ?, ?, ?, ?, ?);
	`

	_, err := db.Exec(query, c.ID, c.URL, c.ParentID, c.Username, c.ProfilePic, c.Comment, c.SentimentScore)
	if err != nil {
		http.Error(w, "Database insert error", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Send back the comment ID that was inserted
	json.NewEncoder(w).Encode(map[string]int{"comment_id": c.ID})
}

