package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

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
	UserID         int    `json:"user_id,omitempty"`
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
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL,
		profile_pic TEXT
	);

	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		user_id INTEGER NOT NULL,
		parent_id INTEGER,
		comment TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		sentiment_score INTEGER DEFAULT 0,
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (parent_id) REFERENCES comments(id)
	);

	CREATE TABLE IF NOT EXISTS comment_likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
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
	r.HandleFunc("/comments", getComments).Methods("GET")
	r.HandleFunc("/replies/{comment_id}", getReplies).Methods("GET")
	r.HandleFunc("/comments", postComment).Methods("POST")

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Get main comments for a URL
func getComments(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "URL parameter is required", http.StatusBadRequest)
		return
	}

	query := `
	SELECT c.id, c.parent_id, c.comment, c.created_at, u.username, u.profile_pic, c.sentiment_score,
	       (SELECT COUNT(*) FROM comments r WHERE r.parent_id = c.id) AS reply_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count
	FROM comments c
	JOIN users u ON c.user_id = u.id
	WHERE c.url = ? AND c.parent_id IS NULL
	ORDER BY c.created_at ASC;
	`

	rows, err := db.Query(query, url)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// Get replies for a specific comment
func getReplies(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	commentID := vars["comment_id"]

	query := `
	SELECT c.id, c.parent_id, c.comment, c.created_at, u.username, u.profile_pic, c.sentiment_score,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 1) AS like_count,
	       (SELECT COUNT(*) FROM comment_likes cl WHERE cl.comment_id = c.id AND cl.is_like = 0) AS dislike_count
	FROM comments c
	JOIN users u ON c.user_id = u.id
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(replies)
}

// Post a new comment
func postComment(w http.ResponseWriter, r *http.Request) {
	var c Comment
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if c.URL == "" || c.UserID == 0 || c.Comment == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	query := `
	INSERT INTO comments (url, user_id, parent_id, comment, sentiment_score) 
	VALUES (?, ?, ?, ?, ?) RETURNING id;
	`

	var commentID int
	err := db.QueryRow(query, c.URL, c.UserID, c.ParentID, c.Comment, c.SentimentScore).Scan(&commentID)
	if err != nil {
		http.Error(w, "Database insert error", http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"comment_id": commentID})
}
