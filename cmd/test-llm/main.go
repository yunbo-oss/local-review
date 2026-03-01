// 简单测试 LLM API 是否可用（Embedding + Chat）
// 用法：go run ./cmd/test-llm  或  make test-llm
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	logcfg "local-review-go/internal/config/log"
	"local-review-go/internal/llm"

	"github.com/sashabaranov/go-openai"
)

func loadEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			if key != "" {
				os.Setenv(key, val)
			}
		}
	}
}

func main() {
	loadEnv()
	logcfg.Init()
	cfg := llm.LoadConfig()
	if cfg.APIKey == "" {
		log.Fatal("请设置 LLM_API_KEY 环境变量")
	}
	embClient, chatClient := llm.NewOpenAIClient(cfg)
	if embClient == nil {
		log.Fatal("Embedding 客户端初始化失败")
	}
	ctx := context.Background()

	// 1. 测试 Embedding（DeepSeek 可能不支持，先试 Chat）
	fmt.Println("=== 测试 Embedding ===")
	vec, err := embClient.Embed(ctx, "你好，这是一段测试文本")
	if err != nil {
		fmt.Printf("Embedding 失败（DeepSeek 可能不支持）: %v\n", err)
		fmt.Println("跳过 Embedding，继续测试 Chat...")
	} else {
		fmt.Printf("Embedding 维度: %d, 前 3 个值: %v\n", len(vec), vec[:3])
	}

	// 2. 测试 Chat（流式）
	fmt.Println("\n=== 测试 Chat（流式）===")
	var content string
	err = chatClient.ChatStream(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "用一句话介绍北京"},
	}, func(chunk string) {
		content += chunk
		fmt.Print(chunk)
	})
	if err != nil {
		log.Fatalf("Chat 失败: %v", err)
	}
	if len(content) == 0 {
		log.Fatal("Chat 返回为空")
	}
	fmt.Println()
	if len(content) > 0 {
		fmt.Println("\n✅ LLM API 配置正确，可用")
	} else {
		fmt.Println("\n⚠️ Chat 返回为空")
	}
	os.Exit(0)
}
