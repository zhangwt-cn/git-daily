package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ChatResponse 结构体用于解析API响应
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
}

// Config 结构体定义，字段与 JSON 结构对应
type Config struct {
	OpenAIBaseURL   string `json:"openai_base_url"`
	OpenAIAPIKey    string `json:"openai_api_key"`
	ModerationModel string `json:"moderation_model"`
	DailyPath       string `json:"daily_path"`
}

var config Config

func main() {
	loadConfig(`.git/hooks/config.json`)

	// 定义日报文件路径
	date := time.Now().Format("2006-01-02")
	dailyReportFile := fmt.Sprintf("%s/git_report_%s.md", config.DailyPath, date)

	// 获取最新提交的 diff 内容
	diffContent, err := getGitDiff()
	if err != nil || strings.TrimSpace(diffContent) == "" {
		fmt.Println("No diff content found. Skipping analysis.")
		return
	}

	// 获取提交信息
	commitMsg, commitHash, commitDate, repoName, err := getGitCommitInfo()
	if err != nil {
		fmt.Println("Failed to get commit info:", err)
		return
	}

	// 检查日报文件是否已存在，没有则创建文件并写入标题
	if _, err := os.Stat(dailyReportFile); os.IsNotExist(err) {
		err = os.WriteFile(dailyReportFile, []byte(fmt.Sprintf("# Daily Git Report - %s\n", date)), 0644)
		if err != nil {
			fmt.Println("Failed to create daily report file:", err)
			return
		}
	}

	// 检查是否已记录该提交
	reportContent, _ := os.ReadFile(dailyReportFile)
	if strings.Contains(string(reportContent), commitHash) {
		fmt.Println("Commit already recorded:", commitHash)
		return
	}

	// 调用 OpenAI API 分析提交信息和 diff 内容
	analysis, err := analyzeGitDiff(commitMsg, diffContent)
	if err != nil || strings.TrimSpace(analysis) == "" {
		fmt.Println("Failed to fetch analysis or analysis is empty.")
		return
	}

	// 追加分析内容到日报文件
	err = appendToReport(dailyReportFile, repoName, commitHash, commitDate, commitMsg, analysis)
	if err != nil {
		fmt.Println("Failed to append to report file:", err)
	}
}

func getGitDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD^", "HEAD")
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// 获取提交信息
func getGitCommitInfo() (string, string, string, string, error) {
	commitMsgCmd := exec.Command("git", "log", "-1", "--pretty=%B")
	commitMsg, err := commitMsgCmd.CombinedOutput()
	if err != nil {
		return "", "", "", "", err
	}

	commitHashCmd := exec.Command("git", "log", "-1", "--pretty=%H")
	commitHash, err := commitHashCmd.CombinedOutput()
	if err != nil {
		return "", "", "", "", err
	}

	commitDate := time.Now().Format("2006-01-02 15:04:05")
	repoName := "git project"
	if dirName, err := os.Getwd(); err == nil {
		repoName = filepath.Base(dirName)
	}

	return strings.TrimSpace(string(commitMsg)), strings.TrimSpace(string(commitHash)), commitDate, strings.TrimSpace(repoName), nil
}

func loadConfig(filename string) {
	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("打开配置文件错误: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			fmt.Println("关闭文件错误: %w", err)
		}
	}(file)

	// 创建 JSON 解码器并解码
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		fmt.Println("解析配置文件错误: %w", err)
	}
}

func analyzeGitDiff(commitMsg, diffContent string) (string, error) {
	apiURL := config.OpenAIBaseURL + "/chat/completions"
	apiKey := config.OpenAIAPIKey
	payload := map[string]interface{}{
		"model": config.ModerationModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一位擅长技术管理沟通的专家。请基于git代码变动内容，生成一份面向领导的工作日报。要突出业务价值和工作成效，用管理视角描述技术变更。\n\n## 分析重点\n1. 将技术变动转化为业务价值：\n   - 功能如何服务业务目标\n   - 变更带来的具体改进\n   - 对用户体验的提升\n   - 对系统性能的优化\n   - 对维护成本的影响\n\n2. 重点关注：\n   - 项目进展和里程碑\n   - 重要功能的完成情况\n   - 系统改进和优化\n   - 问题修复的效果\n   - 潜在的业务影响\n\n## 输出要求\n1. 使用管理层易于理解的语言\n2. 突出业务价值和实际效果\n3. 避免过多技术细节\n4. 注意工作表述的积极性\n5. 体现主动性和规划性\n\n## 输出格式\n今日工作进展：\n1. [重要进展/完成的功能]\n   - 业务价值说明\n   - 具体改进效果\n   - （如有）后续规划\n\n## 示例\n\n输入：\n```diff\ndiff --git a/src/services/search.js b/src/services/search.js\n--- a/src/services/search.js\n+++ b/src/services/search.js\n@@ -15,6 +15,14 @@ class SearchService {\n+  async searchWithCache(keyword) {\n+    const cacheKey = `search:${keyword}`;\n+    const cached = await cache.get(cacheKey);\n+    if (cached) return cached;\n+    const result = await this.search(keyword);\n+    await cache.set(cacheKey, result, 3600);\n+    return result;\n+  }\n+\n   async search(keyword) {\n-    return await db.query(keyword);\n+    return await this.searchWithCache(keyword);\n   }\n}\n\ndiff --git a/src/services/product.js b/src/services/product.js\n--- a/src/services/product.js\n+++ b/src/services/product.js\n@@ -8,6 +8,10 @@ class ProductService {\n+    if (product.stock < 10) {\n+      await notificationService.notify('stock-warning', {\n+        productId: product.id,\n+        currentStock: product.stock\n+      });\n+    }\n   }\n }\n```\n\n相关commit信息：\n```\nperf: implement search caching\nfeat: add low stock notification\n```\n\n输出：\n今日工作进展：\n1. 优化系统搜索性能\n   - 实现智能缓存机制，显著提升用户搜索响应速度\n   - 预计可减少50%以上的数据库查询压力\n   - 已完成开发和测试，准备在下一版本发布\n\n2. 完善库存管理预警\n   - 新增低库存自动预警功能，支持及时补货决策\n   - 有效预防断货风险，提升库存周转效率\n   - 系统自动监控，无需人工干预\n"},
			{"role": "user", "content": fmt.Sprintf("提交信息:\n'%s'\n\n代码变动:\n'%s'\n\nProvide a detailed analysis:", commitMsg, diffContent)},
		},
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP Status: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var openAIResp ChatResponse
	err = json.Unmarshal(body, &openAIResp)
	if err != nil || len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("invalid response from OpenAI")
	}

	return openAIResp.Choices[0].Message.Content, nil
}

func appendToReport(filePath, repoName, commitHash, commitDate, commitMsg, analysis string) error {
	content, _ := os.ReadFile(filePath)
	updatedContent := string(content)

	repoHeader := fmt.Sprintf("### Repo: %s", repoName)
	entry := fmt.Sprintf("**Commit Hash:** %s\n**Date:** %s\n**Message:** %s\n**Analysis:**\n%s\n\n---\n", commitHash, commitDate, commitMsg, analysis)

	if strings.Contains(updatedContent, repoHeader) {
		updatedContent = strings.Replace(updatedContent, repoHeader, repoHeader+"\n"+entry, 1)
	} else {
		updatedContent += fmt.Sprintf("\n%s\n%s", repoHeader, entry)
	}

	return os.WriteFile(filePath, []byte(updatedContent), 0644)
}
