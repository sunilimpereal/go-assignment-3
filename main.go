package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type CollectRequest struct {
	ProjectGithubURL string `json:"project_github_url"`
	BuildCommand     string `json:"build_command"`
	BuildOutDir      string `json:"build_out_dir"`
}

type BuildEvent struct {
	BuildID string `json:"build_id"`
	Status  string `json:"status"`
	// Add more fields as needed, such as timestamps, etc.
}

type BuildResponse struct {
	BuildID string `json:"build_id"`
}

var db *sql.DB

func main() {
	// Initialize database connection
	var err error
	db, err = sql.Open("postgres", "postgres://postgres:password@localhost:5432/builds?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Initialize Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal(err)
	}

	// Initialize HTTP server
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/collect", CollectHandler).Methods("POST")
	r.HandleFunc("/api/v1/build-events/{buildID}", BuildEventsHandler).Methods("GET")
	r.HandleFunc("/api/v1/build/{buildID}", BuildStatusHandler).Methods("GET")

	log.Fatal(http.ListenAndServe(":8080", r))
}

func CollectHandler(w http.ResponseWriter, r *http.Request) {
	var collectRequest CollectRequest
	if err := json.NewDecoder(r.Body).Decode(&collectRequest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Generate a unique build ID
	buildID := uuid.New().String()

	// Send build information to wf-code-builder via Kafka
	// Assuming Kafka integration code here

	// Save build information to PostgreSQL
	if err := saveBuildInfo(buildID, collectRequest.ProjectGithubURL, collectRequest.BuildCommand, collectRequest.BuildOutDir); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the build ID to the user
	buildResponse := BuildResponse{BuildID: buildID}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildResponse)
}

func BuildEventsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract build ID from request parameters
	vars := mux.Vars(r)
	buildID := vars["buildID"]

	// Fetch build events from Redis queue based on build ID
	// Process build events and return to the user
}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	// Extract build ID from request parameters
	vars := mux.Vars(r)
	buildID := vars["buildID"]

	// Query build status from PostgreSQL
	status, err := getBuildStatus(buildID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return build status to the user
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func saveBuildInfo(buildID, projectURL, buildCommand, buildOutDir string) error {
	_, err := db.Exec("INSERT INTO builds (build_id, project_url, build_command, build_out_dir) VALUES ($1, $2, $3, $4)",
		buildID, projectURL, buildCommand, buildOutDir)
	if err != nil {
		return err
	}
	return nil
}

func getBuildStatus(buildID string) (string, error) {
	var status string
	err := db.QueryRow("SELECT status FROM builds WHERE build_id = $1", buildID).Scan(&status)
	if err != nil {
		return "", err
	}
	return status, nil
}

func buildAndDeploy(buildID, buildCommand, buildOutDir string, dockerClient *client.Client) error {
	// Create a context with timeout for the build process
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build the Docker image with the given build command
	buildCtx, err := os.Open(buildOutDir)
	if err != nil {
		return err
	}
	defer buildCtx.Close()

	buildOptions := types.ImageBuildOptions{
		Context:    buildCtx,
		Dockerfile: "Dockerfile",
		Remove:     true,
		Tags:       []string{buildID},
	}

	buildResp, err := dockerClient.ImageBuild(ctx, buildCtx, buildOptions)
	if err != nil {
		return err
	}
	defer buildResp.Body.Close()

	// Extract build logs and save to database
	logBytes, err := io.ReadAll(buildResp.Body)
	if err != nil {
		return err
	}

	// Save build logs to database
	if err := saveBuildLogs(buildID, string(logBytes)); err != nil {
		return err
	}

	// Push the built image to Docker registry
	// Assuming Docker registry authentication and push implementation here

	// Run the Docker container with the built image
	containerOptions := types.ContainerCreateConfig{
		Name: buildID,
	}
	resp, err := dockerClient.ContainerCreate(ctx, &containerOptions)
	if err != nil {
		return err
	}

	// Start the container
	if err := dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	// Return the deployed port to the user
	info, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return err
	}

	port := ""
	for _, p := range info.NetworkSettings.Ports {
		port = strconv.Itoa(int(p[0].HostPort))
	}

	return nil
}
