package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func sendRequest(request string) {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Connection error:", err)
		return
	}
	defer conn.Close()

	conn.Write([]byte(request))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	fmt.Println() 
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	
	for {
		fmt.Println("\n=== Z  Client ===")
		fmt.Println("Choose request type:")
		fmt.Println("1. ZGET")
		fmt.Println("2. ZPUT")
		fmt.Println("3. Exit")
		fmt.Print("action: ")

		choiceStr, _ := reader.ReadString('\n')
		choiceStr = strings.TrimSpace(choiceStr)
		
		var choice int
		fmt.Sscanf(choiceStr, "%d", &choice)

		if choice == 3 {
			fmt.Println("Exiting program...")
			break
		}

		var request string

		switch choice {
		case 1:
			// ZGET 
			fmt.Print("Enter path (e.g., /index.html): ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)

			request = fmt.Sprintf("ZGET %s Z/1.0\nHost: localhost\n\n", path)
			sendRequest(request)
			
		case 2:
			// ZPUT 
			fmt.Print("Enter path (e.g., /newfile.html): ")
			path, _ := reader.ReadString('\n')
			path = strings.TrimSpace(path)

			fmt.Print("Enter content (text) to upload: ")
			content, _ := reader.ReadString('\n')
			content = strings.TrimSpace(content)

			request = fmt.Sprintf("ZPUT %s Z/1.0\nContent-Length: %d\n\n%s", path, len(content), content)
			sendRequest(request)
			
		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}