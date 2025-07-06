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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/joho/godotenv"
)

type SiteConfig struct {
	Title  string `json:"title"`
	Domain string `json:"domain"`
}

type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

type ZServer struct {
	githubToken string
	repoURL     string
	username    string
	repoName    string
	publicDir   string
	sitesDir    string
	httpClient  *http.Client
}

func NewZServer() *ZServer {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: .env file not found, GitHub features disabled")
	}

	repoURL := os.Getenv("REPO_URL")
	username := os.Getenv("GITHUB_USERNAME")
	
	return &ZServer{
		githubToken: os.Getenv("GITHUB_TOKEN"),
		repoURL:     repoURL,
		username:    username,
		repoName:    extractRepoName(repoURL),
		publicDir:   "./public",
		sitesDir:    "./sites",
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func extractRepoName(repoURL string) string {
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		repoName := parts[len(parts)-1]
		repoName = strings.TrimSuffix(repoName, ".git")
		return repoName
	}
	return ""
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

	// Check if this is a domain-based request (ends with .z)
	if strings.HasSuffix(path, ".z") {
		s.serveDomainSiteFromGitHub(conn, path)
		return
	}

	// Handle regular file serving
	s.serveRegularFile(conn, path)
}

func (s *ZServer) serveDomainSiteFromGitHub(conn net.Conn, domain string) {
	if s.githubToken == "" || s.username == "" || s.repoName == "" {
		conn.Write([]byte("Z/1.0 500 GitHub not configured\n\n"))
		return
	}

	// Find the site folder by domain
	siteFolder, err := s.findSiteFolderByDomain(domain)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("Z/1.0 404 Domain Not Found: %v\n\n", err)))
		return
	}

	// Fetch the raw HTML content
	htmlContent, err := s.fetchRawHTML(siteFolder)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("Z/1.0 500 Fetch Error: %v\n\n", err)))
		return
	}

	// Return raw HTML content directly (no headers)
	conn.Write([]byte(htmlContent))
}

func (s *ZServer) findSiteFolderByDomain(requestedDomain string) (string, error) {
	// Construct expected folder name using the same logic as sanitizeDomainName
	expectedFolderName := s.sanitizeDomainName(requestedDomain)
	
	// Check if the folder exists directly
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/sites/%s", 
		s.username, s.repoName, expectedFolderName)
	
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "token "+s.githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Folder exists, return it
		return expectedFolderName, nil
	} else if resp.StatusCode == 404 {
		// Folder doesn't exist
		return "", fmt.Errorf("no site found with domain: %s (expected folder: %s)", requestedDomain, expectedFolderName)
	} else {
		// Other error
		return "", fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}
}

func (s *ZServer) fetchRawHTML(siteFolder string) (string, error) {
	// Use raw.githubusercontent.com for direct file access
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/sites/%s/index.html", 
		s.username, s.repoName, siteFolder)

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}

	// Add authorization header for private repos
	if s.githubToken != "" {
		req.Header.Set("Authorization", "token "+s.githubToken)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTML file not found: %d", resp.StatusCode)
	}

	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(htmlBytes), nil
}

func (s *ZServer) serveRegularFile(conn net.Conn, path string) {
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
	conn.Write(content)
}

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
	githubURL := fmt.Sprintf("https://%s.github.io/%s/sites/%s/", s.username, s.repoName, folderName)
	response := fmt.Sprintf("Z/1.0 200 Deployed\nSite: %s\nDomain: %s\nLocation: %s\n\n", config.Title, strings.ToLower(config.Domain), githubURL)	
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
	domain = strings.ToLower(domain)
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
	auth := &githttp.BasicAuth{
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
		Auth: &githttp.BasicAuth{
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
	fmt.Println("  ZGET  - Retrieve files and serve GitHub sites")
	fmt.Println("  ZDEPLOY - Deploy sites to GitHub")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  ZGET filename.html    - Serve regular file")
	fmt.Println("  ZGET domain.z         - Serve GitHub site content")
	fmt.Println("")
	fmt.Printf("GitHub Repository: https://github.com/%s/%s\n", server.username, server.repoName)

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