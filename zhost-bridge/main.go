package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
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

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("error loading .env file")
		return
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	repoURL := os.Getenv("REPO_URL")
	username := os.Getenv("GITHUB_USERNAME")
	
	if githubToken == "" || repoURL == "" || username == "" {
		fmt.Println("Missing in .env")
		return
	}

	//  upload directory from user
	fmt.Print("Enter the path to site directory(containing your files and config.json): ")
	var uploadDir string
	fmt.Scanln(&uploadDir)

	if uploadDir == "" {
		fmt.Println("No directory specified")
		return
	}

	// Validate the upload directory and config
	config, err := validateUploadDirectory(uploadDir)
	if err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		return
	}

	fmt.Printf("Site Title: %s\n", config.Title)
	fmt.Printf("Domain: %s\n", config.Domain)
	fmt.Print("Proceed with upload? (y/n): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("Upload cancelled")
		return
	}

	//  unique folder name based on domain and timestamp
	timestamp := time.Now().Format("20060102-150405")
	folderName := fmt.Sprintf("%s-%s", sanitizeDomainName(config.Domain), timestamp)
	
	repoDir := "./tmp-repo"
	
	// clone or pull repo
	r, err := cloneOrPullRepo(repoURL, repoDir, username, githubToken)
	if err != nil {
		fmt.Printf("Repository operation failed: %v\n", err)
		return
	}

	// target directory in repo
	targetDir := filepath.Join(repoDir, "sites", folderName)
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		fmt.Printf("Failed to create target directory: %v\n", err)
		return
	}

	// copy files from upload directory to target directory
	err = copyDirectory(uploadDir, targetDir)
	if err != nil {
		fmt.Printf("Failed to copy files: %v\n", err)
		return
	}

	// commit and push changes
	err = commitAndPush(r, folderName, config, username, githubToken)
	if err != nil {
		fmt.Printf("Failed to commit and push: %v\n", err)
		return
	}

	fmt.Println("Site uploaded successfully!")
	fmt.Printf("Site folder: sites/%s\n", folderName)
	fmt.Printf("Config: %+v\n", config)
	
	// extract repo name from URL for GitHub Pages link
	repoName := extractRepoName(repoURL)
	if repoName != "" {
		fmt.Printf("GitHub Pages URL: https://%s.github.io/%s/sites/%s/\n", username, repoName, folderName)
	}
}

func validateUploadDirectory(uploadDir string) (*SiteConfig, error) {
	// Check if directory exists
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory does not exist: %s", uploadDir)
	}

	// Check if config.json exists
	configPath := filepath.Join(uploadDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config.json not found in directory: %s", uploadDir)
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

func sanitizeDomainName(domain string) string {
	// Remove protocol if present
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	
	// Replace invalid characters with hyphens
	domain = strings.ReplaceAll(domain, ".", "-")
	domain = strings.ReplaceAll(domain, "/", "-")
	domain = strings.ReplaceAll(domain, ":", "-")
	domain = strings.ReplaceAll(domain, " ", "-")
	
	// Remove multiple consecutive hyphens
	for strings.Contains(domain, "--") {
		domain = strings.ReplaceAll(domain, "--", "-")
	}
	
	// Trim hyphens from ends
	domain = strings.Trim(domain, "-")
	
	// Limit length
	if len(domain) > 30 {
		domain = domain[:30]
	}
	
	return domain
}

func cloneOrPullRepo(repoURL, repoDir, username, githubToken string) (*git.Repository, error) {
	auth := &http.BasicAuth{
		Username: username,
		Password: githubToken,
	}

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Println("Cloning repository...")
		r, err := git.PlainClone(repoDir, false, &git.CloneOptions{
			URL:  repoURL,
			Auth: auth,
		})
		if err != nil {
			return nil, fmt.Errorf("clone failed: %v", err)
		}
		return r, nil
	} else {
		fmt.Println("Opening existing repository...")
		r, err := git.PlainOpen(repoDir)
		if err != nil {
			return nil, fmt.Errorf("open failed: %v", err)
		}

		fmt.Println("Pulling latest changes...")
		w, _ := r.Worktree()
		err = w.Pull(&git.PullOptions{Auth: auth})
		if err != nil && err.Error() != "already up-to-date" {
			return nil, fmt.Errorf("pull failed: %v", err)
		}
		return r, nil
	}
}

func copyDirectory(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Calculate destination path
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, d.Type())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, 0644)
	})
}

func commitAndPush(r *git.Repository, folderName string, config *SiteConfig, username, githubToken string) error {
	w, _ := r.Worktree()

	fmt.Println("Adding files to git...")
	_, err := w.Add("sites/" + folderName)
	if err != nil {
		return fmt.Errorf("add failed: %v", err)
	}

	fmt.Println("Committing changes...")
	commitMsg := fmt.Sprintf("Add site: %s (domain: %s)", config.Title, config.Domain)
	_, err = w.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Z Protocol Bridge",
			Email: "bridge@zprotocol.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("commit failed: %v", err)
	}

	fmt.Println("Pushing to git")
	err = r.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: username,
			Password: githubToken,
		},
	})
	if err != nil {
		return fmt.Errorf("push failed: %v", err)
	}

	return nil
}

func extractRepoName(repoURL string) string {
	// etract repo name from site URL
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		repoName := parts[len(parts)-1]
		// remove .git if present
		repoName = strings.TrimSuffix(repoName, ".git")
		return repoName
	}
	return ""
}