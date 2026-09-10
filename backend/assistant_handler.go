package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const assistantSystemPrompt = `你是家具电商无限画布中的 AI Assistant。你的职责包括：
1. 产品外观、结构、材质与场景参考分析；
2. 两图或多图的差异比较；
3. 为 Nano Banana 与 GPT Image 生成可执行的电商生图提示词；
4. 按用户要求修改、压缩或扩写提示词。

遵守以下规则：只根据用户提供的文字与图片作答；看不到或无法确认的结构、尺寸、材质、性能必须明确标注“未知/待确认”，不得编造商品事实。生成提示词时，清楚写出需要保持不变的产品特征、参考图分工、场景、构图、镜头、光影、材质真实性和输出约束。回复使用用户当前语言，内容简洁、可直接执行。`

type AssistantChatRequest struct {
	Messages        []AssistantMessage `json:"messages"`
	SelectedImages  []string           `json:"selected_images"`
	CanvasID        string             `json:"canvas_id"`
	SelectedNodeIDs []string           `json:"selected_node_ids"`
}

type AssistantMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type assistantChatPayload struct {
	Model    string                `json:"model"`
	Messages []assistantAPIMessage `json:"messages"`
}

type assistantAPIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type assistantContentPart struct {
	Type     string             `json:"type"`
	Text     string             `json:"text,omitempty"`
	ImageURL *assistantImageURL `json:"image_url,omitempty"`
}

type assistantImageURL struct {
	URL string `json:"url"`
}

func (h *Handler) ChatWithAssistant(c *gin.Context) {
	var req AssistantChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式不正确"})
		return
	}
	model := strings.TrimSpace(h.assistantModel)
	if model == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "图片分析模型尚未配置"})
		return
	}
	if h.vectorEngine == nil || !h.vectorEngine.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrVectorEngineNotConfigured.Error()})
		return
	}

	messages, err := normalizeAssistantMessages(req.Messages)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resolvedImages, err := h.resolveSelectedNodeImages(req.CanvasID, req.SelectedNodeIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selectedImages := mergeReferenceImages(resolvedImages, req.SelectedImages)
	apiMessages, err := buildAssistantAPIMessages(messages, selectedImages, h.assetUploadDir())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	payload := assistantChatPayload{
		Model:    model,
		Messages: apiMessages,
	}
	responseBody, err := h.vectorEngine.PostJSON(c.Request.Context(), "/v1/chat/completions", payload)
	if err != nil {
		h.log.Error("VectorEngine assistant failed", "error", err)
		var apiErr *VectorEngineAPIError
		if errors.As(err, &apiErr) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "AI Assistant 调用失败", "upstream_status": apiErr.StatusCode})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI Assistant 调用失败"})
		return
	}

	content, err := assistantResponseText(responseBody)
	if err != nil {
		h.log.Error("invalid assistant response", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI Assistant 未返回有效文本"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content, "model_profile": "assistant"})
}

func buildAssistantAPIMessages(messages []AssistantMessage, selectedImages []string, uploadDir string) ([]assistantAPIMessage, error) {
	if len(selectedImages) > maxReferenceImages {
		return nil, errors.New("Assistant 视觉上下文最多支持 8 张图片")
	}
	result := make([]assistantAPIMessage, 0, len(messages)+1)
	result = append(result, assistantAPIMessage{Role: "system", Content: assistantSystemPrompt})
	lastIndex := len(messages) - 1
	for index, message := range messages {
		if index != lastIndex || message.Role != "user" || len(selectedImages) == 0 {
			result = append(result, assistantAPIMessage{Role: message.Role, Content: message.Content})
			continue
		}
		parts := []assistantContentPart{{
			Type: "text",
			Text: message.Content + "\n\n以下图片按顺序对应图1、图2……，请严格按编号分析。",
		}}
		for imageIndex, reference := range selectedImages {
			inline, err := loadReferenceImage(reference, uploadDir)
			if err != nil {
				return nil, fmt.Errorf("选中图片%d无效: %w", imageIndex+1, err)
			}
			parts = append(parts, assistantContentPart{
				Type: "image_url",
				ImageURL: &assistantImageURL{
					URL: "data:" + inline.MIMEType + ";base64," + inline.Data,
				},
			})
		}
		result = append(result, assistantAPIMessage{Role: message.Role, Content: parts})
	}
	return result, nil
}

func normalizeAssistantMessages(input []AssistantMessage) ([]AssistantMessage, error) {
	if len(input) == 0 {
		return nil, errors.New("消息不能为空")
	}
	if len(input) > 20 {
		input = input[len(input)-20:]
	}
	result := make([]AssistantMessage, 0, len(input))
	for _, message := range input {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		content := strings.TrimSpace(message.Content)
		if role != "user" && role != "assistant" {
			return nil, errors.New("消息角色不合法")
		}
		if content == "" {
			continue
		}
		if len([]rune(content)) > 20000 {
			return nil, errors.New("单条消息过长")
		}
		result = append(result, AssistantMessage{Role: role, Content: content})
	}
	if len(result) == 0 || result[len(result)-1].Role != "user" {
		return nil, errors.New("最后一条消息必须是用户输入")
	}
	return result, nil
}

func assistantResponseText(body []byte) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", errors.New("响应中没有 choices")
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("响应文本为空")
	}
	return content, nil
}
