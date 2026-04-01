package main

import (
	"encoding/hex"
	"regexp"
)

type ContractAnalyzer struct {
	rules []analysisRule
}

type analysisRule struct {
	name        string
	pattern     *regexp.Regexp
	severity    string
	description string
}

type AnalysisReport struct {
	Valid           bool      `json:"valid"`
	Score           int       `json:"score"`
	Issues          []Issue   `json:"issues"`
	Warnings        []Warning `json:"warnings"`
	Recommendations []string  `json:"recommendations"`
}

type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

type Warning struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func NewContractAnalyzer() *ContractAnalyzer {
	rules := []analysisRule{
		{
			name:        "external_call",
			pattern:     regexp.MustCompile(`(?i)(call|delegatecall|staticcall|assembly\s*\{\s*.*call)`),
			severity:    "high",
			description: "External contract call detected",
		},
		{
			name:        "selfdestruct",
			pattern:     regexp.MustCompile(`(?i)(selfdestruct|suicide)`),
			severity:    "critical",
			description: "Self-destruct instruction found",
		},
		{
			name:        "tx_origin",
			pattern:     regexp.MustCompile(`(?i)tx\.origin`),
			severity:    "medium",
			description: "tx.origin usage detected (vulnerability prone)",
		},
		{
			name:        "unprotected_suicide",
			pattern:     regexp.MustCompile(`(?i)(selfdestruct|suicide).*require|if.*selfdestruct`),
			severity:    "medium",
			description: "Unprotected self-destruct",
		},
		{
			name:        "reentrancy",
			pattern:     regexp.MustCompile(`(?i)call\.value.*\.\s*\{[^}]*\}`),
			severity:    "high",
			description: "Potential reentrancy vulnerability (call.value pattern)",
		},
		{
			name:        "unlocked_ether",
			pattern:     regexp.MustCompile(`(?i)balance.*>.*0.*return|this\.balance`),
			severity:    "medium",
			description: "Potential unlocked ether",
		},
		{
			name:        "ether_transfer_loop",
			pattern:     regexp.MustCompile(`(?i)for.*\{.*\.transfer|for.*\{.*\.send`),
			severity:    "high",
			description: "Ether transfer in loop - potential DOS",
		},
		{
			name:        "access_control",
			pattern:     regexp.MustCompile(`(?i)(onlyOwner|require.*msg\.sender\s*==.*owner|require.*owner)`),
			severity:    "low",
			description: "Access control detected",
		},
		{
			name:        "state_variable",
			pattern:     regexp.MustCompile(`(?i)^\s*(uint|int|address|bool|bytes)[0-9]*\s+\w+\s*;`),
			severity:    "info",
			description: "State variable declaration",
		},
		{
			name:        "storage_write",
			pattern:     regexp.MustCompile(`(?i)(\w+)\s*\[.*\]\s*=|sstore`),
			severity:    "info",
			description: "Storage write operation",
		},
		{
			name:        "timestamp",
			pattern:     regexp.MustCompile(`(?i)(block\.timestamp|now)`),
			severity:    "low",
			description: "Timestamp usage - can be manipulated by miners",
		},
		{
			name:        "block_number",
			pattern:     regexp.MustCompile(`(?i)block\.number`),
			severity:    "low",
			description: "Block number dependency",
		},
		{
			name:        "assembly",
			pattern:     regexp.MustCompile(`(?i)assembly\s*\{`),
			severity:    "medium",
			description: "Inline assembly detected",
		},
		{
			name:        "abi_encode",
			pattern:     regexp.MustCompile(`(?i)(abi\.encode|abi\.encodePacked|abi\.encodeWithSignature)`),
			severity:    "info",
			description: "ABI encoding function usage",
		},
		{
			name:        "ecrecover",
			pattern:     regexp.MustCompile(`(?i)ecrecover`),
			severity:    "medium",
			description: "ECDSA signature verification (ecrecover)",
		},
	}

	return &ContractAnalyzer{
		rules: rules,
	}
}

func (a *ContractAnalyzer) Analyze(data string) AnalysisReport {
	report := AnalysisReport{
		Valid:           true,
		Score:           100,
		Issues:          []Issue{},
		Warnings:        []Warning{},
		Recommendations: []string{},
	}

	if data == "" {
		report.Recommendations = append(report.Recommendations, "No data to analyze")
		return report
	}

	hexData := data
	if _, err := hex.DecodeString(data); err == nil {
		if decoded, err := hex.DecodeString(data); err == nil {
			hexData = string(decoded)
		}
	}

	for _, rule := range a.rules {
		matches := rule.pattern.FindAllString(hexData, -1)
		if len(matches) > 0 {
			issue := Issue{
				Type:        rule.name,
				Severity:    rule.severity,
				Description: rule.description,
				Location:    "data field",
			}
			report.Issues = append(report.Issues, issue)

			switch rule.severity {
			case "critical":
				report.Score -= 50
				report.Valid = false
			case "high":
				report.Score -= 25
			case "medium":
				report.Score -= 10
			case "low":
				report.Score -= 5
			}
		}
	}

	if report.Score < 0 {
		report.Score = 0
	}

	if report.Score >= 80 {
		report.Valid = true
	} else if report.Score >= 50 {
		report.Valid = true
		report.Warnings = append(report.Warnings, Warning{
			Type:        "medium_score",
			Description: "Transaction has medium risk score",
		})
	}

	report.Recommendations = a.generateRecommendations(report.Issues)

	return report
}

func (a *ContractAnalyzer) generateRecommendations(issues []Issue) []string {
	var recommendations []string

	hasReentrancy := false
	hasSelfDestruct := false
	hasExternalCall := false

	for _, issue := range issues {
		switch issue.Type {
		case "reentrancy":
			hasReentrancy = true
		case "selfdestruct":
			hasSelfDestruct = true
		case "external_call":
			hasExternalCall = true
		}
	}

	if hasReentrancy {
		recommendations = append(recommendations,
			"Use reentrancy guard (e.g., OpenZeppelin's ReentrancyGuard)",
			"Apply checks-effects-interactions pattern",
			"Use pull payment pattern instead of direct transfers",
		)
	}

	if hasSelfDestruct {
		recommendations = append(recommendations,
			"Ensure self-destruct is only callable by authorized address",
			"Consider time locks for critical operations",
			"Add multi-sig requirement for destructive actions",
		)
	}

	if hasExternalCall {
		recommendations = append(recommendations,
			"Validate return values from external calls",
			"Consider using call over transfer for flexibility",
			"Implement proper error handling",
		)
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No specific recommendations")
	}

	return recommendations
}

func (a *ContractAnalyzer) AnalyzeTransaction(tx Transaction) AnalysisReport {
	data := tx.Data
	if data == "" {
		return AnalysisReport{
			Valid:           true,
			Score:           100,
			Issues:          []Issue{},
			Warnings:        []Warning{},
			Recommendations: []string{"No contract data to analyze"},
		}
	}

	return a.Analyze(data)
}
