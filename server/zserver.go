package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/joho/godotenv"
)

type SiteConfig struct {
	Title  string `json:"title"`
	Domain string `json:"domain"`
}

type ZServer struct {
	githubToken string
	repoURL     string
	username    string
	publicDir   string
}

func NewZServer() *ZServer {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: .env file not found, GitHub features disabled")
	}

	return &ZServer{
		githubToken: os.Getenv("GITHUB_TOKEN"),
		repoURL:     os.Getenv("REPO_URL"),
		username:    os.Getenv("GITHUB_USERNAME"),
		publicDir:   "./public",
	}
}

func (s *ZServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Read error:", err)
		return
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		conn.Write([]byte("Z/1.0 400 Bad Request\n\n"))
		return
	}

	method := parts[0]
	path := parts[1]

	// Clean path
	path = strings.TrimPrefix(path, "/")

	switch method {
	case "ZGET":
		s.serveFile(conn, path)
	case "ZDEPLOY":
		s.handleDeploy(conn, reader, path)
	default:
		conn.Write([]byte("Z/1.0 405 Method Not Allowed\n\n"))
	}
}

func (s *ZServer) serveFile(conn net.Conn, path string) {
	if path == "" {
		path = "index.html"
	}

	// Handle `.z` as an alias for `.html`
	if strings.HasSuffix(path, ".z") {
		path = strings.TrimSuffix(path, ".z") + ".html"
	}

	// Add .html if no extension and .html file exists in public
	if filepath.Ext(path) == "" {
		possibleHTML := path + ".html"
		fullHTMLPath := filepath.Join(s.publicDir, possibleHTML)
		if _, err := os.Stat(fullHTMLPath); err == nil {
			path = possibleHTML
		}
	}

	fullPath := filepath.Join(s.publicDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		conn.Write([]byte("Z/1.0 404 Not Found\n\n"))
		return
	}

	// Determine content type
	contentType := "text/plain"
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html":
		contentType = "text/html"
	case ".css":
		contentType = "text/css"
	case ".js":
		contentType = "application/javascript"
	case ".json":
		contentType = "application/json"
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	}

	headers := fmt.Sprintf("Z/1.0 200 OK\nContent-Type: %s\nContent-Length: %d\n\n", contentType, len(content))
	conn.Write([]byte(headers))
	conn.Write(content)
}

// func (s *ZServer) handlePut(conn net.Conn, reader *bufio.Reader, path string) {
// 	headers := make(map[string]string)
// 	for {
// 		line, _ := reader.ReadString('\n')
// 		if line == "\n" || line == "\r\n" {
// 			break
// 		}
// 		parts := strings.SplitN(line, ":", 2)
// 		if len(parts) == 2 {
// 			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
// 		}
// 	}

// 	length := 0
// 	fmt.Sscanf(headers["Content-Length"], "%d", &length)

// 	content := make([]byte, length)
// 	_, err := reader.Read(content)
// 	if err != nil {
// 		conn.Write([]byte("Z/1.0 500 Internal Server Error\n\n"))
// 		return
// 	}

// 	fmt.Printf("Content received for file %s: %s\n", path, string(content))

// 	// Ensure public directory exists
// 	fullPath := filepath.Join(s.publicDir, path)
// 	dir := filepath.Dir(fullPath)
// 	os.MkdirAll(dir, 0755)

// 	err = os.WriteFile(fullPath, content, 0644)
// 	if err != nil {
// 		conn.Write([]byte("Z/1.0 500 Write Failed\n\n"))
// 		return
// 	}

// 	conn.Write([]byte("Z/1.0 201 Created\n\n"))
// }
func unzipData(data []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	for _, f := range reader.File {
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// method: Handle site deployment to GitHub
func (s *ZServer) handleDeploy(conn net.Conn, reader *bufio.Reader, _ string) {
	if s.githubToken == "" || s.repoURL == "" || s.username == "" {
		conn.Write([]byte("Z/1.0 500 GitHub not configured\n\n"))
		return
	}

	headers := make(map[string]string)
	for {
		line, _ := reader.ReadString('\n')
		if line == "\n" || line == "\r\n" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	length := 0
	fmt.Sscanf(headers["Content-Length"], "%d", &length)
	contentType := headers["Content-Type"]

	data := make([]byte, length)
	_, err := io.ReadFull(reader, data)
	if err != nil {
		conn.Write([]byte("Z/1.0 500 Internal Server Error\n\n"))
		return
	}

	// Prepare a temp dir to unzip to
	tempDir, err := os.MkdirTemp("", "zdeploy-")
	if err != nil {
		conn.Write([]byte("Z/1.0 500 Internal Server Error\n\n"))
		return
	}
	defer os.RemoveAll(tempDir) // cleanup

	if contentType == "application/zip" {
		err = unzipData(data, tempDir)
		if err != nil {
			conn.Write([]byte(fmt.Sprintf("Z/1.0 400 Unzip Failed: %v\n\n", err)))
			return
		}
	} else {
		conn.Write([]byte("Z/1.0 400 Unsupported Content-Type\n\n"))
		return
	}

	// Validate uploaded folder structure inside tempDir
	config, err := s.validateUploadDirectory(tempDir)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("Z/1.0 400 Validation Failed: %v\n\n", err)))
		return
	}

	fmt.Printf("Deploying site: %s, domain: %s\n", config.Title, config.Domain)

	// Deploy site from tempDir
	err = s.deploySite(tempDir, config)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("Z/1.0 500 Deploy Failed: %v\n\n", err)))
		return
	}

	folderName := s.sanitizeDomainName(config.Domain)
	repoName := s.extractRepoName(s.repoURL)
	githubURL := fmt.Sprintf("https://%s.github.io/%s/sites/%s/", s.username, repoName, folderName)

	response := fmt.Sprintf("Z/1.0 200 Deployed\nSite: %s\nDomain: %s\nLocation: %s\n\n", config.Title, config.Domain, githubURL)
	conn.Write([]byte(response))
}

func (s *ZServer) deploySite(localFolderPath string, config *SiteConfig) error {
	folderName := s.sanitizeDomainName(config.Domain)
	repoDir := filepath.Join(os.TempDir(), "z-protocol-repo")

	// Clone or pull repo
	r, err := s.cloneOrPullRepo(repoDir)
	if err != nil {
		return err
	}

	// Create target directory in repo
	targetDir := filepath.Join(repoDir, "sites", folderName)
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return err
	}

	// Copy all files from local folder to target directory
	err = s.copyDirectory(localFolderPath, targetDir)
	if err != nil {
		return err
	}

	// Commit and push
	return s.commitAndPush(r, folderName, config)
}

func (s *ZServer) sanitizeDomainName(domain string) string {
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.ReplaceAll(domain, ".", "-")
	domain = strings.ReplaceAll(domain, "/", "-")
	domain = strings.ReplaceAll(domain, ":", "-")
	domain = strings.ReplaceAll(domain, " ", "-")

	for strings.Contains(domain, "--") {
		domain = strings.ReplaceAll(domain, "--", "-")
	}

	domain = strings.Trim(domain, "-")

	if len(domain) > 30 {
		domain = domain[:30]
	}

	return domain
}

func (s *ZServer) cloneOrPullRepo(repoDir string) (*git.Repository, error) {
	auth := &http.BasicAuth{
		Username: s.username,
		Password: s.githubToken,
	}

	if stat, err := os.Stat(repoDir); err == nil && stat.IsDir() {
		if r, err := git.PlainOpen(repoDir); err == nil {
			fmt.Println("Using existing repository...")
			w, err := r.Worktree()
			if err == nil {
				w.Pull(&git.PullOptions{Auth: auth})
			}
			return r, nil
		} else {
			os.RemoveAll(repoDir)
		}
	}

	fmt.Println("Cloning repository...")
	r, err := git.PlainClone(repoDir, false, &git.CloneOptions{
		URL:  s.repoURL,
		Auth: auth,
	})
	return r, err
}

func (s *ZServer) copyDirectory(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, d.Type())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, 0644)
	})
}

func (s *ZServer) commitAndPush(r *git.Repository, folderName string, config *SiteConfig) error {
	w, err := r.Worktree()
	if err != nil {
		return err
	}

	_, err = w.Add("sites/" + folderName)
	if err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("Add site: %s (domain: %s)", config.Title, config.Domain)
	_, err = w.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Z Protocol Bridge",
			Email: "bridge@zprotocol.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}

	return r.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: s.username,
			Password: s.githubToken,
		},
	})
}

func (s *ZServer) validateUploadDirectory(uploadDir string) (*SiteConfig, error) {
	// Check if directory exists
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory does not exist: %s", uploadDir)
	}

	// Check if config.json exists
	configPath := filepath.Join(uploadDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config.json not found in directory: %s", uploadDir)
	}

	// Check if index.html exists
	indexPath := filepath.Join(uploadDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("index.html not found in directory: %s", uploadDir)
	}

	// Read and parse config.json
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %v", err)
	}

	var config SiteConfig
	err = json.Unmarshal(configData, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %v", err)
	}

	// Validate required fields
	if strings.TrimSpace(config.Title) == "" {
		return nil, fmt.Errorf("config.json must contain a non-empty 'title' field")
	}

	if strings.TrimSpace(config.Domain) == "" {
		return nil, fmt.Errorf("config.json must contain a non-empty 'domain' field")
	}

	return &config, nil
}

func (s *ZServer) extractRepoName(repoURL string) string {
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		repoName := parts[len(parts)-1]
		repoName = strings.TrimSuffix(repoName, ".git")
		return repoName
	}
	return ""
}

func main() {
	server := NewZServer()

	// Ensure public directory exists
	os.MkdirAll(server.publicDir, 0755)

	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Server start failed:", err)
		os.Exit(1)
	}

	fmt.Println("Z Protocol Server running on port 8080...")
	fmt.Println("Supported methods:")
	fmt.Println("  ZGET  - Retrieve files")
	fmt.Println("  ZDEPLOY - Deploy sites to GitHub")

	if server.githubToken != "" {
		fmt.Println("GitHub integration: ENABLED")
	} else {
		fmt.Println("GitHub integration: DISABLED (no .env config)")
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go server.handleConnection(conn)
	}
}
