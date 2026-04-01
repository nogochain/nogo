package ai

import (
	"regexp"
	"strings"
)

type ContractAnalyzer struct {
	rules []SecurityRule
}

type SecurityRule struct {
	Name           string
	Severity       string
	Pattern        string
	Description    string
	Recommendation string
}

type AnalysisResult struct {
	IsSafe      bool
	Issues      []SecurityIssue
	Summary     string
	SafeOpcodes []string
	RiskScore   float64
}

type SecurityIssue struct {
	Type           string
	Severity       string
	Location       string
	Description    string
	Recommendation string
}

var contractSecurityRules = []SecurityRule{
	{
		Name:           "Reentrancy",
		Severity:       "high",
		Pattern:        "(?i)(call|callvalue|delegatecall).*balance.*transfer",
		Description:    "Potential reentrancy vulnerability detected",
		Recommendation: "Use checks-effects-interactions pattern or reentrancy guard",
	},
	{
		Name:           "Integer Overflow",
		Severity:       "high",
		Pattern:        "(?i)(add|sub|mul).*(overflow|underflow)",
		Description:    "Potential integer overflow/underflow",
		Recommendation: "Use SafeMath or Solidity 0.8+ checked math",
	},
	{
		Name:           "Selfdestruct",
		Severity:       "high",
		Pattern:        "(?i)selfdestruct|suicide",
		Description:    "Contract can be destroyed",
		Recommendation: "Ensure destruction is properly authorized",
	},
	{
		Name:           "Unchecked Call Return",
		Severity:       "medium",
		Pattern:        "(?i)call.*returnvalue.*ignored",
		Description:    "Call return value not checked",
		Recommendation: "Always check return values",
	},
	{
		Name:           "Delegatecall",
		Severity:       "critical",
		Pattern:        "(?i)delegatecall",
		Description:    "Using delegatecall with untrusted code is dangerous",
		Recommendation: "Ensure delegatecall target is trusted and immutable",
	},
	{
		Name:           "External Call",
		Severity:       "medium",
		Pattern:        "(?i)external.*call|call\\(",
		Description:    "External call detected",
		Recommendation: "Limit gas forwarded to prevent reentrancy",
	},
	{
		Name:           "tx.origin Usage",
		Severity:       "medium",
		Pattern:        "(?i)tx\\.origin",
		Description:    "Using tx.origin for authorization is vulnerable to phishing",
		Recommendation: "Use msg.sender instead",
	},
	{
		Name:           "Access Control",
		Severity:       "high",
		Pattern:        "(?i)(owner|admin).*require",
		Description:    "Missing or weak access control",
		Recommendation: "Implement proper access control with OpenZeppelin Ownable",
	},
	{
		Name:           "Floating Pragma",
		Severity:       "low",
		Pattern:        "(?i)pragma.*\\^",
		Description:    "Floating pragma can lead to unexpected behavior",
		Recommendation: "Lock pragma version",
	},
	{
		Name:           "Integer Division",
		Severity:       "medium",
		Pattern:        "(?i)/.*division",
		Description:    "Integer division may truncate",
		Recommendation: "Use precise arithmetic or check for zero divisor",
	},
}

func NewContractAnalyzer() *ContractAnalyzer {
	rules := make([]SecurityRule, len(contractSecurityRules))
	copy(rules, contractSecurityRules)
	return &ContractAnalyzer{
		rules: rules,
	}
}

func (ca *ContractAnalyzer) Analyze(bytecode string) *AnalysisResult {
	result := &AnalysisResult{
		IsSafe:      true,
		Issues:      []SecurityIssue{},
		SafeOpcodes: []string{},
	}

	bytecodeLower := strings.ToLower(bytecode)

	vulnerableOpcodes := map[string]string{
		"callcode":     "delegatecall-like behavior",
		"delegate":     "delegatecall execution",
		"selfdestruct": "contract destruction",
		"suicide":      "contract destruction",
	}

	for opcode, issue := range vulnerableOpcodes {
		if strings.Contains(bytecodeLower, opcode) {
			result.IsSafe = false
			result.Issues = append(result.Issues, SecurityIssue{
				Type:           opcode,
				Severity:       "high",
				Location:       "bytecode",
				Description:    "Vulnerable opcode detected: " + issue,
				Recommendation: "Review and audit the contract",
			})
			result.RiskScore += 40
		}
	}

	safeOpcodes := map[string]string{
		"push":  "Push operation",
		"pop":   "Pop operation",
		"add":   "Addition",
		"sub":   "Subtraction",
		"mul":   "Multiplication",
		"div":   "Division",
		"stop":  "Stop execution",
		"swap":  "Stack swap",
		"dup":   "Stack duplication",
		"jump":  "Conditional jump",
		"jumpi": "Conditional jump",
		"store": "Storage write",
		"load":  "Storage read",
	}

	for opcode := range safeOpcodes {
		if strings.Contains(bytecodeLower, opcode) {
			result.SafeOpcodes = append(result.SafeOpcodes, opcode)
		}
	}

	result.Summary = "Contract analysis complete"
	if len(result.Issues) == 0 {
		result.Summary = "No obvious vulnerabilities detected in bytecode"
	}

	if result.RiskScore > 70 {
		result.IsSafe = false
	} else if result.RiskScore > 30 {
		result.IsSafe = false
	}

	return result
}

func (ca *ContractAnalyzer) AnalyzeSource(sourceCode string) *AnalysisResult {
	result := &AnalysisResult{
		IsSafe:      true,
		Issues:      []SecurityIssue{},
		SafeOpcodes: []string{},
	}

	if sourceCode == "" {
		result.Summary = "No source code provided"
		return result
	}

	for _, rule := range ca.rules {
		re := regexp.MustCompile(rule.Pattern)
		if re.MatchString(sourceCode) {
			result.Issues = append(result.Issues, SecurityIssue{
				Type:           rule.Name,
				Severity:       rule.Severity,
				Location:       "source",
				Description:    rule.Description,
				Recommendation: rule.Recommendation,
			})

			switch rule.Severity {
			case "critical":
				result.RiskScore += 50
			case "high":
				result.RiskScore += 30
			case "medium":
				result.RiskScore += 15
			case "low":
				result.RiskScore += 5
			}
		}
	}

	if len(result.Issues) == 0 {
		result.Summary = "No security issues detected"
		result.IsSafe = true
	} else {
		result.IsSafe = result.RiskScore < 50
		result.Summary = "Found issues requiring review"
	}

	return result
}

func (ca *ContractAnalyzer) GetSecurityScore(bytecode, sourceCode string) float64 {
	result := ca.Analyze(bytecode)
	if sourceCode != "" {
		sourceResult := ca.AnalyzeSource(sourceCode)
		result.Issues = append(result.Issues, sourceResult.Issues...)
		result.RiskScore = (result.RiskScore + sourceResult.RiskScore) / 2
	}
	return result.RiskScore
}

func (ca *ContractAnalyzer) CheckCommonVulnerabilities(bytecode, sourceCode string) map[string]bool {
	checks := map[string]bool{
		"reentrancy_safe":       true,
		"overflow_safe":         true,
		"access_control_secure": true,
		"tx_origin_safe":        true,
		"unchecked_calls_safe":  true,
	}

	result := ca.Analyze(bytecode)
	if sourceCode != "" {
		sourceResult := ca.AnalyzeSource(sourceCode)
		result.Issues = append(result.Issues, sourceResult.Issues...)
	}

	for _, issue := range result.Issues {
		switch issue.Type {
		case "Reentrancy":
			checks["reentrancy_safe"] = false
		case "Integer Overflow":
			checks["overflow_safe"] = false
		case "Access Control":
			checks["access_control_secure"] = false
		case "tx.origin Usage":
			checks["tx_origin_safe"] = false
		case "Unchecked Call Return":
			checks["unchecked_calls_safe"] = false
		}
	}

	return checks
}
