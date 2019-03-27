package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	// tpl stores the parsed frontend html template
	tpl *template.Template
)

// guestbookEntry represents the message object returned from the backend API.
type guestbookEntry struct {
	Author  string    `json:"author"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
}

// main starts a frontend server and connects to the backend.
func main() {
	// GUESTBOOK_API_ADDR environment variable is provided in guestbook-frontend.deployment.yaml.
	backendAddr := os.Getenv("GUESTBOOK_API_ADDR")
	if backendAddr == "" {
		log.Fatal("GUESTBOOK_API_ADDR environment variable not specified")
	}

	// PORT environment variable is provided in guestbook-frontend.deployment.yaml.
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable not specified")
	}

	t, err := template.New("").Funcs(map[string]interface{}{
		"since": sinceDate,
	}).ParseGlob("templates/*.tpl")
	if err != nil {
		log.Fatalf("could not parse templates: %+v", err)
	}
	tpl = t

	fe := &frontendServer{backendAddr: backendAddr}
	http.HandleFunc("/", fe.homeHandler)
	http.HandleFunc("/post", fe.postHandler)

	log.Printf("frontend server listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("server listen error: %+v", err)
	}
}

type frontendServer struct {
	backendAddr string
}

// homeHandler handles GET requests to /.
func (f *frontendServer) homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("received request: %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		http.Error(w, fmt.Sprintf("only GET requests are supported (got %s)", r.Method), http.StatusMethodNotAllowed)
		return
	} else if r.URL.Path != "/" {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	log.Printf("querying backend for entries")
	resp, err := http.Get(fmt.Sprintf("http://%s/messages", f.backendAddr))
	if err != nil {
		http.Error(w, fmt.Sprintf("querying backend failed: %+v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read response body: %+v", err), http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("got status code %d from the backend: %s", resp.StatusCode, string(body)), http.StatusInternalServerError)
		return
	}

	log.Printf("parsing backend response into json")
	var v []guestbookEntry
	if err := json.Unmarshal(body, &v); err != nil {
		log.Printf("WARNING: failed to decode json from the api: %+v input=%q", err, string(body))
		http.Error(w,
			fmt.Sprintf("could not decode json response from the api: %+v", err),
			http.StatusInternalServerError)
		return
	}

	log.Printf("retrieved %d messages from the backend api", len(v))
	if err := tpl.ExecuteTemplate(w, "home", map[string]interface{}{
		"messages": v,
	}); err != nil {
		log.Printf("WARNING: failed to render html template: %+v", err)
	}
}

// postHandler handles POST requests to /messages.
func (f *frontendServer) postHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("received request: %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodPost {
		http.Error(w, "only POST requests are supported", http.StatusMethodNotAllowed)
		return
	}

	if err := f.saveMessage(r.FormValue("name"), r.FormValue("message")); err != nil {
		http.Error(w, fmt.Sprintf("failed to save message: %+v", err), http.StatusBadRequest)
		return
	} else {
		// redirect to homepage
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// saveMessage makes a request to the backend to persist the message.
func (f *frontendServer) saveMessage(author, message string) error {
	if author == "" {
		return errors.New("Please enter your name.")
	} else if message == "" {
		return errors.New("Please write a message.")
	}

	entry := guestbookEntry{
		Author:  author,
		Message: message,
	}
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to serialize message into json: %+v", err)
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/messages", f.backendAddr),
		"application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("backend returned failure: %+v", err)
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code from backend: %d %v", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()
	return nil
}

// sinceDate is used in the html template to display human-friendly dates.
func sinceDate(t time.Time) string { return time.Since(t).Truncate(time.Second).String() }
