package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hatrot/trot-web/ai"
	"github.com/hatrot/trot-web/notify"

	"cloud.google.com/go/storage"
)

// Initialize variable ==========================================
var TimeZone = time.FixedZone("Asia/Tokyo", 9*60*60)
var SitNam string = "trot.co.jp"
var BktNam string = "trot-web.appspot.com"

// Common function ==============================================
func main() {
	// Initialize handler
	http.HandleFunc("/_ah/health", healthCheckHandler)
	http.HandleFunc("/_ah/warmup", warmupHandler)
	http.HandleFunc("/api/chat", chatHandler)
	http.HandleFunc("/api/contact", contactHandler)
	http.HandleFunc("/", indexHandler)

	// Set listen port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	// Start Listening
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

type chatRequest struct {
	Question string       `json:"question"`
	History  []ai.Message `json:"history"`
}

type chatResponse struct {
	Reply    string `json:"reply"`
	Provider string `json:"provider"`
	Error    string `json:"error,omitempty"`
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	// CORS Headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(chatResponse{Error: "Invalid JSON body"})
		return
	}

	if strings.TrimSpace(req.Question) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(chatResponse{Error: "Question is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	service := ai.GetService()
	reply, providerName, err := service.Ask(ctx, req.History, req.Question)
	if err != nil {
		log.Printf("chatHandler error: %v", err)
		// Fallback to Mock if API Key is missing for demonstration
		if strings.Contains(err.Error(), "GEMINI_API_KEY is not configured") {
			mock := ai.NewMockProvider()
			reply, _ = mock.GenerateReply(ctx, "", req.History, req.Question)
			providerName = mock.Name()
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(chatResponse{
				Error:    err.Error(),
				Provider: providerName,
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(chatResponse{
		Reply:    reply,
		Provider: providerName,
	})
}

type contactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

type contactResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	// CORS Headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(contactResponse{Status: "error", Error: "Invalid JSON body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	content := strings.TrimSpace(req.Content)
	if name == "" || content == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(contactResponse{Status: "error", Error: "お名前とご相談内容は必須です"})
		return
	}

	// Dispatch notification via unified Notifier (Slack / SendGrid / Log)
	notifier := notify.NewNotifier()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := notifier.Send(ctx, notify.ContactMessage{
		Name:    name,
		Email:   strings.TrimSpace(req.Email),
		Content: content,
		Source:  req.Source,
	}); err != nil {
		log.Printf("Warning: Notification dispatch error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(contactResponse{
		Status:  "ok",
		Message: "サポート担当へ送信されました。担当者より折り返しご連絡差し上げます。",
	})
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "ok")
}

func warmupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "warmup done")
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://www.youtube.com/c/ryo2ch", 302)
}

// Main function ==================================================
func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Initialize variable
	cas := "public, max-age=300"
	obj := r.URL.Path[1:]
	fil := obj
	sep := strings.LastIndex(obj, "/")
	if sep >= 0 {
		fil = obj[sep:]
	}
	if fil == "/" {
		obj = obj[:len(obj)-1]
	}
	chk := strings.LastIndex(fil, ".") < 0
	// Change path
	switch {
	case obj == "":
		obj = "www/index.html"
	case chk:
		obj = "www/" + obj + "/index.html"
	default:
		obj = "www/" + obj
	}

	// 1. ローカルファイルが存在する場合は直接配信 (ローカル開発・テスト用)
	if info, err := os.Stat(obj); err == nil && !info.IsDir() {
		w.Header().Set("Cache-Control", cas)
		http.ServeFile(w, r, obj)
		return
	}

	// 2. GCS からの読み込み (本番環境用)
	ctx, cancel := context.WithTimeout(r.Context(), time.Second*30)
	defer cancel()
	clt, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("indexHandler: storage client error: %v", err)
		http.NotFound(w, r)
		return
	}
	defer clt.Close()

	// Read Content
	red, err := clt.Bucket(BktNam).Object(obj).NewReader(ctx)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	defer red.Close()

	// Output
	w.Header().Set("Cache-Control", cas)
	w.Header().Set("Content-Type", red.ContentType())
	w.WriteHeader(http.StatusOK)
	io.Copy(w, red)
}
