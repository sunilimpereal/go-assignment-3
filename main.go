package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
)

// RequestBody represents the structure of the JSON request body
type RequestBody struct {
	ProjectGithubURL string `json:"project_github_url"`
	BuildCommand     string `json:"build_command"`
	BuildOutDir      string `json:"build_out_dir"`
}

func main() {
	http.HandleFunc("/api/v1/collect", collectHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func collectHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the request method is POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode JSON request body
	var reqBody RequestBody
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if reqBody.ProjectGithubURL == "" || reqBody.BuildCommand == "" || reqBody.BuildOutDir == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Clone the repository
	err = cloneRepository(reqBody.ProjectGithubURL)
	if err != nil {
		http.Error(w, "Failed to clone repository", http.StatusInternalServerError)
		return
	}

	// Execute build command
	output, err := executeBuildCommand(reqBody.BuildCommand, reqBody.BuildOutDir)
	if err != nil {
		http.Error(w, "Failed to execute build command: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Build successful", "output": output})
}

func cloneRepository(repoURL string) error {
	cmd := exec.Command("git", "clone", repoURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func executeBuildCommand(buildCommand, buildOutDir string) (string, error) {
	cmd := exec.Command("sh", "-c", buildCommand)
	cmd.Dir = buildOutDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
