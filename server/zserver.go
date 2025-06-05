package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func handleConnection(conn net.Conn) {
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

    // path validity check, need to start with /
	if !strings.HasPrefix(path, "/") {
		conn.Write([]byte("Z/1.0 400 Bad Request\n\n"))
		return
	}

	// cleaning  path removing any extra slashes
	path = strings.TrimPrefix(path, "/")

	switch method {
	case "ZGET":
		serveFile(conn, path)
	case "ZPUT":
		handlePut(conn, reader, path)
	default:
		conn.Write([]byte("Z/1.0 405 Method Not Allowed\n\n"))
	}
}

func serveFile(conn net.Conn, path string) {
	content, err := os.ReadFile("../public/" + path)
	if err != nil {
		conn.Write([]byte("Z/1.0 404 Not Found\n\n"))
		return
	}

	headers := fmt.Sprintf("Z/1.0 200 OK\nContent-Type: text/html\nContent-Length: %d\n\n", len(content))
	conn.Write([]byte(headers))
	conn.Write(content)
}

func handlePut(conn net.Conn, reader *bufio.Reader, path string) {
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

    content := make([]byte, length)
    _, err := reader.Read(content)
    if err != nil {
        conn.Write([]byte("Z/1.0 500 Internal Server Error\n\n"))
        return
    }

    fmt.Printf("Content received for file %s: %s\n", path, string(content))

    err = os.WriteFile("../public/"+path, content, 0644)
    if err != nil {
        conn.Write([]byte("Z/1.0 500 Write Failed\n\n"))
        return
    }

    conn.Write([]byte("Z/1.0 201 Created\n\n"))
}


func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Server start failed:", err)
		os.Exit(1)
	}
	fmt.Println("Z Server on port 8080...")

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}
