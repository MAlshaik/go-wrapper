package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/websocket/v2"
    "github.com/joho/godotenv"

)

const openAIURL = "https://api.openai.com/v1/chat/completions"
var openAIKey string

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type OpenAIRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Stream   bool      `json:"stream"`
}

type OpenAIResponse struct {
    Choices []struct {
        Delta struct {
            Content string `json:"content"`
        } `json:"delta"`
    } `json:"choices"`
}

type WebSocketMessage struct {
    Text string `json:"text"`
}

var (
    clients = make(map[*websocket.Conn]bool)
)

func main() {
    err := godotenv.Load()
    if err != nil {
        fmt.Println("Error loading .env file")
    }

    openAIKey = os.Getenv("OPENAI_API_KEY")
    if openAIKey == "" {
        fmt.Println("OPENAI_API_KEY is required")
        return
    }
    app := fiber.New()
    app.Static("/", "./static")
    app.Get("/", handleHome)
    app.Get("/ws", websocket.New(handleWebSocket))
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    fmt.Println("Server running on port", port)
    app.Listen(":" + port)
}

func handleHome(c *fiber.Ctx) error {
    return c.SendFile("./static/index.html")
}

func handleWebSocket(c *websocket.Conn) {
    clients[c] = true
    defer delete(clients, c)
    for {
        var msg WebSocketMessage
        err := c.ReadJSON(&msg)
        if err != nil {
            fmt.Println("error:", err)
            break
        }
        go streamResponse(msg.Text, c)
    }
}

func streamResponse(message string, conn *websocket.Conn) {
    openAIReq := OpenAIRequest{
        Model: "gpt-4-mini",
        Messages: []Message{
            {Role: "user", Content: message},
        },
        Stream: true,
    }
    reqBody, _ := json.Marshal(openAIReq)
    req, _ := http.NewRequest("POST", openAIURL, strings.NewReader(string(reqBody)))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+openAIKey)
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Println("Error calling OpenAI API:", err)
        return
    }
    defer resp.Body.Close()
    reader := bufio.NewReader(resp.Body)
    isFirstToken := true
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            if err == io.EOF {
                break
            }
            fmt.Println("Error reading response:", err)
            break
        }
        line = strings.TrimSpace(line)
        if line == "" || line == "data: [DONE]" {
            continue
        }
        line = strings.TrimPrefix(line, "data: ")
        var aiResp OpenAIResponse
        err = json.Unmarshal([]byte(line), &aiResp)
        if err != nil {
            continue
        }
        if len(aiResp.Choices) > 0 {
            content := aiResp.Choices[0].Delta.Content
            if content != "" {
                if isFirstToken {
                    conn.WriteJSON(WebSocketMessage{Text: "AI: " + content})
                    isFirstToken = false
                } else {
                    conn.WriteJSON(WebSocketMessage{Text: content})
                }
            }
        }
    }
}
