package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cohesion-org/deepseek-go"
	"github.com/viswesh-swaminathan/ashvattha/internal/marketdata"
)

const (
	systemPrompt = `You are an expert cryptocurrency trading advisor with deep knowledge of technical analysis, risk management, and market dynamics. Your role is to analyze the provided account information and market data to provide actionable trading recommendations.

When analyzing the data, consider:
- Current positions and their risk/reward profiles
- Technical indicators across multiple timeframes
- Market momentum and trend strength
- Risk management and portfolio balance
- Entry/exit opportunities based on technical setups

Provide your response in JSON format with the following structure:
{
  "analysis": "Brief overall market and portfolio analysis",
  "recommendations": [
    {
      "symbol": "trading pair symbol",
      "action": "BUY/SELL/HOLD/CLOSE",
      "reasoning": "detailed reasoning for this recommendation",
      "confidence": 0.0-1.0,
      "suggested_leverage": "1x/10x/20x/40x",
	  "suggested_size": "position size in base currency or null",
      "suggested_entry": price or null,
      "suggested_stop_loss": price or null,
      "suggested_take_profit": price or null,
      "risk_assessment": "LOW/MEDIUM/HIGH"
    }
  ],
  "portfolio_suggestions": "Overall portfolio management advice",
  "risk_warnings": ["any important risk warnings or concerns"]
}`
)

type (
	Prompt struct {
		accountInfo string
		marketdata  []*marketdata.MarketData
		client      *deepseek.Client
	}

	// Structured response for the application
	Recommendation struct {
		Symbol              string  `json:"symbol"`
		Action              string  `json:"action"`
		Reasoning           string  `json:"reasoning"`
		Confidence          float64 `json:"confidence"`
		SuggestedLeverage   string  `json:"suggested_leverage,omitempty"`
		SuggestedSize       string  `json:"suggested_size,omitempty"`
		SuggestedEntry      float64 `json:"suggested_entry,omitempty"`
		SuggestedStopLoss   float64 `json:"suggested_stop_loss,omitempty"`
		SuggestedTakeProfit float64 `json:"suggested_take_profit,omitempty"`
		RiskAssessment      string  `json:"risk_assessment"`
	}

	DeepSeekResponse struct {
		Analysis             string           `json:"analysis"`
		Recommendations      []Recommendation `json:"recommendations"`
		PortfolioSuggestions string           `json:"portfolio_suggestions"`
		RiskWarnings         []string         `json:"risk_warnings"`
		RawResponse          string           `json:"-"` // Store raw response for debugging
		TokensUsed           int              `json:"-"` // Store token usage
	}
)

func NewPrompt(apiKey string, accountInfo string, marketdata []*marketdata.MarketData) *Prompt {
	return &Prompt{
		accountInfo: accountInfo,
		marketdata:  marketdata,
		client:      deepseek.NewClient(apiKey),
	}
}

// Prompt sends account and market data to DeepSeek API and returns structured trading recommendations
func (p *Prompt) Prompt(userQuery string) (*DeepSeekResponse, error) {
	// Build the prompt with account info and market data
	promptText, err := p.buildPrompt(userQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Call DeepSeek API using the SDK
	response, err := p.callDeepSeekAPI(promptText)
	if err != nil {
		return nil, fmt.Errorf("failed to call DeepSeek API: %w", err)
	}

	// Parse the response
	result, err := p.parseResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (p *Prompt) buildPrompt(userQuery string) (string, error) {
	var sb strings.Builder

	// Add user query if provided
	if userQuery != "" {
		sb.WriteString("User Question: ")
		sb.WriteString(userQuery)
		sb.WriteString("\n\n")
	}

	// Add account information
	sb.WriteString("=== ACCOUNT INFORMATION ===\n")
	sb.WriteString(p.accountInfo)
	sb.WriteString("\n\n")

	// Add market data for all symbols
	sb.WriteString("=== MARKET DATA ===\n")
	for _, data := range p.marketdata {
		sb.WriteString(data.Format())
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func (p *Prompt) callDeepSeekAPI(promptText string) (*deepseek.ChatCompletionResponse, error) {
	// Build request using SDK types
	request := &deepseek.ChatCompletionRequest{
		Model: deepseek.DeepSeekReasoner,
		Messages: []deepseek.ChatCompletionMessage{
			{
				Role:    deepseek.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    deepseek.ChatMessageRoleUser,
				Content: promptText,
			},
		},
	}

	// Create context with timeout (reasoning model can take several minutes)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start a progress indicator goroutine
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("Still waiting for DeepSeek response... (reasoning models can take several minutes)")
			case <-done:
				return
			}
		}
	}()

	// Send request using SDK
	log.Println("Sending request to DeepSeek API...")
	response, err := p.client.CreateChatCompletion(ctx, request)
	done <- true // Stop progress indicator

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	log.Println("Received response from DeepSeek API")
	return response, nil
}

func (p *Prompt) parseResponse(apiResponse *deepseek.ChatCompletionResponse) (*DeepSeekResponse, error) {
	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("no choices in API response")
	}

	content := apiResponse.Choices[0].Message.Content

	// Try to extract JSON from the response (in case it's wrapped in markdown code blocks)
	jsonContent := extractJSON(content)

	var result DeepSeekResponse
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		// If parsing fails, return error with the raw content for debugging
		return nil, fmt.Errorf("failed to parse JSON response: %w\nRaw content: %s", err, content)
	}

	// Store metadata
	result.RawResponse = content
	result.TokensUsed = apiResponse.Usage.TotalTokens

	return &result, nil
}

// extractJSON attempts to extract JSON from markdown code blocks or returns the content as-is
func extractJSON(content string) string {
	// Look for JSON in markdown code blocks
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.Index(content[start:], "```")
		if end != -1 {
			return strings.TrimSpace(content[start : start+end])
		}
	}

	// Look for JSON in generic code blocks
	if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.Index(content[start:], "```")
		if end != -1 {
			extracted := strings.TrimSpace(content[start : start+end])
			// Check if it starts with { which indicates JSON
			if strings.HasPrefix(extracted, "{") {
				return extracted
			}
		}
	}

	// Return as-is if no code blocks found
	return strings.TrimSpace(content)
}
