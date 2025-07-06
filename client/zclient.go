package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func zipFolder(folderPath string) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create the header with relative path inside zip
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Directories are implicitly created by files inside them, so skip
			return nil
		}

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = relPath
		fh.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(fh)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		zipWriter.Close()
		return nil, err
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func sendRequest(request []byte) {
	conn, err := net.Dial("tcp", "0.tcp.in.ngrok.io:11291")  // TCP NGROK UTX
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(request)
	if err != nil {
		fmt.Println("Write error:", err)
		return
	}

	// read full response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		fmt.Println("Read error:", err)
		return
	}

	fmt.Println(string(buf[:n]))
}


func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=== Z Protocol Client ===")
		fmt.Println("Choose request type:")
		fmt.Println("1. ZGET - Get file")
		fmt.Println("2. ZDEPLOY - Deploy site to GitHub then to YetiZ")
		fmt.Println("3. ZVALIDATE - To check validity of domain")
		fmt.Println("4. Exit")
		fmt.Print("Action: ")

		choiceStr, _ := reader.ReadString('\n')
		choiceStr = strings.TrimSpace(choiceStr)

		var choice int
		fmt.Sscanf(choiceStr, "%d", &choice)

		if choice == 4 {
			fmt.Println("Exiting program...")
			break
		}

		switch choice {
		case 1:
			// ZGET
			fmt.Print("Enter domain name  (e.g. nishant.z): ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)
			request := fmt.Sprintf("ZGET %s Z/1.0\nHost: localhost\n\n", path)
			sendRequest([]byte(request))

		case 2:
			// ZDEPLOY
			fmt.Print("Enter local folder path (containing index.html and config.json): ")
			folderPath, _ := reader.ReadString('\n')
			folderPath = strings.TrimSpace(folderPath)

			if folderPath == "" {
				fmt.Println("No folder path provided")
				continue
			}

			zipData, err := zipFolder(folderPath)
			if err != nil {
				fmt.Println("Error zipping folder:", err)
				continue
			}

			request := fmt.Sprintf(
				"ZDEPLOY / Z/1.0\nContent-Length: %d\nContent-Type: application/zip\n\n",
				len(zipData),
			)

			reqBytes := append([]byte(request), zipData...)
			sendRequest(reqBytes)
		case 3:
			// ZVALIDATE
			fmt.Print("Enter domain to check (e.g. nishant): ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)
			request := fmt.Sprintf("ZVALIDATE %s Z/1.0\n\n", path)
			sendRequest([]byte(request))
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}
