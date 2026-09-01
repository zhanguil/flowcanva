package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type gptImageGenerationRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	N       int    `json:"n"`
	Size    string `json:"size"`
	Format  string `json:"format"`
	Quality string `json:"quality"`
}

type gptImageResult struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

func (h *Handler) generateGPTImage(c *gin.Context, req VectorImageRequest, profile string) {
	if utf8.RuneCountInString(req.Prompt) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "GPT Image 2 提示词最多支持 1000 个字符"})
		return
	}

	model := h.imageModelForProfile(profile)
	count := allowImageCount(req.N)
	size := gptImageSize(req.AspectRatio, req.ImageSize)

	var (
		responseBody []byte
		err          error
	)
	if len(req.ReferenceImages) == 0 {
		payload := gptImageGenerationRequest{
			Model: model, Prompt: req.Prompt, N: count, Size: size, Format: "png", Quality: "auto",
		}
		responseBody, err = h.vectorEngine.PostJSON(c.Request.Context(), "/v1/images/generations", payload)
	} else {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if formErr := writeGPTImageEditForm(writer, req, model, count, size, h.assetUploadDir()); formErr != nil {
			_ = writer.Close()
			c.JSON(http.StatusBadRequest, gin.H{"error": formErr.Error()})
			return
		}
		if closeErr := writer.Close(); closeErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "构建 GPT Image 2 请求失败"})
			return
		}
		responseBody, err = h.vectorEngine.PostMultipart(c.Request.Context(), "/v1/images/edits", writer.FormDataContentType(), &body)
	}
	if err != nil {
		h.writeImageGenerationError(c, err)
		return
	}

	generated, err := normalizeGPTImages(responseBody)
	if err != nil {
		h.log.Error("invalid VectorEngine GPT image response", "model_profile", profile, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 未返回有效图片"})
		return
	}
	if len(generated) < count {
		h.log.Error("VectorEngine GPT image count mismatch", "requested", count, "returned", len(generated))
		c.JSON(http.StatusBadGateway, gin.H{"error": "VectorEngine 返回图片数量不足"})
		return
	}

	assets, err := h.persistGeneratedImages(generated[:count], profile)
	if err != nil {
		h.log.Error("persist generated images failed", "model_profile", profile, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成图片保存失败"})
		return
	}
	if err := h.persistNodeGeneratedOutputs(req.CanvasID, req.NodeID, assets); err != nil {
		h.log.Error("persist generation node output failed", "task_id", req.TaskID, "node_id", req.NodeID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生图节点输出保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": assets, "model_profile": profile})
}

func writeGPTImageEditForm(writer *multipart.Writer, req VectorImageRequest, model string, count int, size, uploadDir string) error {
	if len(req.ReferenceImages) > maxReferenceImages {
		return fmt.Errorf("参考图片最多支持 %d 张", maxReferenceImages)
	}
	for i, reference := range req.ReferenceImages {
		inline, err := loadReferenceImage(reference, uploadDir)
		if err != nil {
			return fmt.Errorf("参考图%d无效: %w", i+1, err)
		}
		raw, err := base64.StdEncoding.DecodeString(inline.Data)
		if err != nil {
			return fmt.Errorf("参考图%d base64 数据损坏", i+1)
		}
		ext, err := imageExtension(inline.MIMEType)
		if err != nil {
			return fmt.Errorf("参考图%d: %w", i+1, err)
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="reference-%d%s"`, i+1, ext))
		header.Set("Content-Type", inline.MIMEType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return fmt.Errorf("创建参考图%d字段: %w", i+1, err)
		}
		if _, err := part.Write(raw); err != nil {
			return fmt.Errorf("写入参考图%d: %w", i+1, err)
		}
	}
	fields := map[string]string{
		"model": model, "prompt": req.Prompt, "n": strconv.Itoa(count), "size": size, "quality": "auto",
	}
	for _, name := range []string{"model", "prompt", "n", "size", "quality"} {
		if err := writer.WriteField(name, fields[name]); err != nil {
			return fmt.Errorf("写入 GPT Image 2 字段 %s: %w", name, err)
		}
	}
	return nil
}

func normalizeGPTImages(body []byte) ([]geminiInlineData, error) {
	var envelope struct {
		Data         json.RawMessage `json:"data"`
		OutputFormat string          `json:"output_format"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("解析响应: %w", err)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil, errors.New("响应中没有 image data")
	}

	var results []gptImageResult
	if envelope.Data[0] == '[' {
		if err := json.Unmarshal(envelope.Data, &results); err != nil {
			return nil, fmt.Errorf("解析图片列表: %w", err)
		}
	} else {
		var result gptImageResult
		if err := json.Unmarshal(envelope.Data, &result); err != nil {
			return nil, fmt.Errorf("解析图片: %w", err)
		}
		results = []gptImageResult{result}
	}

	images := make([]geminiInlineData, 0, len(results))
	for _, result := range results {
		encoded := strings.TrimSpace(result.B64JSON)
		mimeType := outputFormatMIME(envelope.OutputFormat)
		if strings.HasPrefix(encoded, "data:") {
			comma := strings.IndexByte(encoded, ',')
			if comma < 0 || !strings.Contains(encoded[:comma], ";base64") {
				continue
			}
			mimeType = strings.TrimPrefix(strings.SplitN(encoded[:comma], ";", 2)[0], "data:")
			encoded = encoded[comma+1:]
		}
		if encoded == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		if mimeType == "" {
			mimeType = http.DetectContentType(raw)
		}
		images = append(images, geminiInlineData{MIMEType: mimeType, Data: encoded})
	}
	if len(images) == 0 {
		return nil, errors.New("响应中没有 base64 图片；临时 URL 不会发送到浏览器")
	}
	return images, nil
}

func outputFormatMIME(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func gptImageSize(ratio, resolution string) string {
	ratio = allowValue(ratio, "1:1", "自适应", "1:1", "4:3", "3:4", "3:2", "2:3", "16:9", "9:16", "5:4", "4:5", "21:9")
	if ratio == "自适应" {
		return "auto"
	}
	resolution = allowValue(strings.ToUpper(resolution), "1K", "1K", "2K", "4K")
	sizes := map[string]map[string]string{
		"1K": {
			"1:1": "1024x1024", "4:3": "1152x864", "3:4": "864x1152", "3:2": "1248x832", "2:3": "832x1248",
			"16:9": "1344x768", "9:16": "768x1344", "5:4": "1120x896", "4:5": "896x1120", "21:9": "1536x656",
		},
		"2K": {
			"1:1": "2048x2048", "4:3": "2048x1536", "3:4": "1536x2048", "3:2": "2048x1360", "2:3": "1360x2048",
			"16:9": "2048x1152", "9:16": "1152x2048", "5:4": "2048x1632", "4:5": "1632x2048", "21:9": "2304x992",
		},
		"4K": {
			"1:1": "2880x2880", "4:3": "3312x2480", "3:4": "2480x3312", "3:2": "3520x2352", "2:3": "2352x3520",
			"16:9": "3840x2160", "9:16": "2160x3840", "5:4": "3200x2560", "4:5": "2560x3200", "21:9": "3840x1648",
		},
	}
	return sizes[resolution][ratio]
}
