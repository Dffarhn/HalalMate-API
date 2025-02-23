package services

import (
	"HalalMate/config/environment"
	"HalalMate/models"
	"context"
	"errors"
	"log"

	"github.com/sashabaranov/go-openai"
)

type OpenAIService struct {
	Client *openai.Client
}

func NewOpenAIService() *OpenAIService {
	client := openai.NewClient(environment.GetOpenAIKey())
	return &OpenAIService{Client: client}
}

// Generate Text using OpenAI GPT
func (s *OpenAIService) GenerateText(req models.OpenAIRequest) (string, error) {
	model := req.Model
	if model == "" {
		model = openai.GPT4Turbo // Default model GPT-4 Turbo
	}

	resp, err := s.Client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: req.SystemPrompt},
				{Role: openai.ChatMessageRoleUser, Content: req.UserPrompt},
			},
		},
	)

	if err != nil {
		log.Println("Error generating text:", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

// Read Text from Image (OCR) using OpenAI Vision
func (s *OpenAIService) ReadTextFromImage(req models.OpenAIRequest) (string, error) {
	if req.ImageURL == "" {
		return "", errors.New("image URL is required")
	}

	resp, err := s.Client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4VisionPreview, // Model untuk membaca teks dari gambar
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: "Extract and return only the text from the image."},
				{Role: openai.ChatMessageRoleUser, Content: req.ImageURL},
			},
		},
	)

	if err != nil {
		log.Println("Error reading text from image:", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}
