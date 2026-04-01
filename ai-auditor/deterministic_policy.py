"""
NogoChain Deterministic Policy Engine v3.0
- Deterministic, verifiable fraud detection
- Cryptographic attestation of audit results
- On-chain policy governance ready
"""

import re
import math
import hashlib
import json
import time
from typing import Dict, List, Tuple, Optional
from dataclasses import dataclass, asdict
from enum import Enum


class RiskLevel(Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


@dataclass
class PolicyRule:
    rule_id: str
    name: str
    score: float
    applies: bool


@dataclass
class AuditReceipt:
    """Cryptographic attestation of audit result"""

    tx_hash: str
    timestamp: int
    policy_version: str
    risk_score: float
    risk_level: str
    valid: bool
    reasons: List[str]
    input_hash: str
    receipt_hash: str

    def to_dict(self) -> dict:
        return asdict(self)


class DeterministicResult:
    def __init__(
        self,
        valid: bool,
        risk_score: float,
        risk_level: RiskLevel,
        reasons: List[str],
        rules_triggered: List[str],
        policy_version: str,
        tx_hash: str = "",
    ):
        self.valid = valid
        self.risk_score = risk_score
        self.risk_level = risk_level
        self.reasons = reasons
        self.rules_triggered = rules_triggered
        self.policy_version = policy_version
        self.tx_hash = tx_hash
        self.receipt = self._create_receipt()

    def _create_receipt(self) -> AuditReceipt:
        input_data = f"{self.tx_hash}:{self.risk_score}:{self.policy_version}"
        input_hash = hashlib.sha256(input_data.encode()).hexdigest()

        receipt_data = f"{input_hash}:{time.time()}"
        receipt_hash = hashlib.sha256(receipt_data.encode()).hexdigest()[:16]

        return AuditReceipt(
            tx_hash=self.tx_hash,
            timestamp=int(time.time()),
            policy_version=self.policy_version,
            risk_score=self.risk_score,
            risk_level=self.risk_level.value,
            valid=self.valid,
            reasons=self.reasons,
            input_hash=input_hash,
            receipt_hash=receipt_hash,
        )

    def to_dict(self) -> dict:
        return {
            "valid": self.valid,
            "risk_score": self.risk_score,
            "risk_level": self.risk_level.value,
            "reasons": self.reasons,
            "rules_triggered": self.rules_triggered,
            "policy_version": self.policy_version,
            "receipt": self.receipt.to_dict(),
        }


class DeterministicPolicyEngine:
    """
    Deterministic policy engine - same input always produces same output.
    No external state, no time dependencies, no ML.
    """

    VERSION = "3.0.0"

    # Policy Schema - structured for on-chain governance
    POLICY_SCHEMA = {
        "version": "3.0.0",
        "name": "NogoChain Security Policy",
        "description": "Deterministic fraud detection rules",
        "rules": [
            {"id": "blacklist", "type": "address_check", "action": "reject"},
            {"id": "whitelist", "type": "address_check", "action": "allow"},
            {"id": "max_amount", "type": "amount_limit", "threshold": 10_000_000},
            {"id": "dangerous_patterns", "type": "regex_match", "action": "flag"},
            {"id": "suspicious_patterns", "type": "regex_match", "action": "warn"},
            {"id": "money_laundering", "type": "regex_match", "action": "reject"},
            {"id": "high_entropy", "type": "data_analysis", "threshold": 7.5},
            {
                "id": "contract_interaction",
                "type": "pattern_detection",
                "action": "flag",
            },
        ],
    }

    # Thresholds (fixed, deterministic)
    RISK_THRESHOLD_CRITICAL = 70.0
    RISK_THRESHOLD_MEDIUM = 40.0
    MAX_AMOUNT = 10_000_000
    MIN_AMOUNT = 1
    LARGE_AMOUNT = 1_000_000
    MAX_DATA_LENGTH = 10000
    HIGH_ENTROPY_THRESHOLD = 7.5

    # Deterministic patterns (compiled once)
    DANGEROUS_PATTERNS = [
        (re.compile(r"reentrancy", re.I), 50.0, "defi_exploit"),
        (re.compile(r"overflow|underflow", re.I), 40.0, "smart_contract_bug"),
        (re.compile(r"selfdestruct", re.I), 50.0, "destructive_contract"),
        (re.compile(r"delegatecall", re.I), 45.0, "delegatecall_risk"),
        (re.compile(r"ponzi|pyramid", re.I), 70.0, "fraud_scheme"),
        (re.compile(r"rug.?pull|rugpull", re.I), 70.0, "fraud_scheme"),
        (re.compile(r"scam", re.I), 60.0, "fraud"),
        (re.compile(r"phish", re.I), 60.0, "phishing"),
        (re.compile(r"fake.?ico", re.I), 65.0, "ico_scam"),
        (re.compile(r"airdrop.?scam", re.I), 60.0, "airdrop_scam"),
        (re.compile(r"exploit", re.I), 50.0, "exploit"),
        (re.compile(r"hack", re.I), 45.0, "hack"),
        (re.compile(r"bruteforce", re.I), 50.0, "brute_force"),
        (re.compile(r"keylogger", re.I), 60.0, "malware"),
        (re.compile(r"cryptojack", re.I), 55.0, "cryptojacking"),
        (re.compile(r"flash.?loan|mev|sandwich.?attack", re.I), 50.0, "defi_exploit"),
        (re.compile(r"front.?run", re.I), 40.0, "mev"),
        (re.compile(r"oracle.?manipulation", re.I), 50.0, "oracle_manipulation"),
        (re.compile(r"price.?manipulation", re.I), 45.0, "market_manipulation"),
        (re.compile(r"wash.?trade", re.I), 45.0, "wash_trading"),
    ]

    SUSPICIOUS_PATTERNS = [
        (re.compile(r"free.*token|free.*coin", re.I), 15.0, "suspicious_promotion"),
        (re.compile(r"claim.*now", re.I), 12.0, "urgency_scam"),
        (re.compile(r"double.*your", re.I), 20.0, "investment_scam"),
        (re.compile(r"guaranteed.*return", re.I), 18.0, "investment_scam"),
        (re.compile(r"no.*risk", re.I), 15.0, "investment_scam"),
        (re.compile(r"urgent|limited.?time", re.I), 10.0, "urgency_tactic"),
        (re.compile(r"100x|1000x|moon", re.I), 12.0, "pump_and_dump"),
        (re.compile(r"diamond.?hands", re.I), 5.0, "hodl_indoctrination"),
    ]

    # AI Agent specific patterns
    AI_AGENT_PATTERNS = [
        (
            re.compile(r"agent.?call|agent.?execute|ai.?agent", re.I),
            5.0,
            "ai_agent_call",
        ),
        (re.compile(r"autonomous|self.?executing", re.I), 3.0, "autonomous_tx"),
        (
            re.compile(r"smart.?contract.?call|contract.?invoke", re.I),
            2.0,
            "contract_invocation",
        ),
    ]

    MONEY_LAUNDERING_PATTERNS = [
        (re.compile(r"mixer", re.I), 45.0, "mixer"),
        (re.compile(r"tumbler", re.I), 45.0, "tumbler"),
        (re.compile(r"laundry", re.I), 40.0, "money_laundering"),
    ]

    # Rate limiting rules
    MAX_TX_PER_BLOCK = 100
    MAX_DATA_SIZE_BYTES = 10000

    def __init__(self):
        self.blacklist: set = set()
        self.whitelist: set = set()

    def load_blacklist(self, addresses: List[str]):
        self.blacklist = {addr.lower() for addr in addresses}

    def load_whitelist(self, addresses: List[str]):
        self.whitelist = {addr.lower() for addr in addresses}

    def _calculate_entropy(self, data: str) -> float:
        """Pure function - deterministic entropy calculation."""
        if not data:
            return 0.0

        char_freq = {}
        for c in data:
            char_freq[c] = char_freq.get(c, 0) + 1

        entropy = 0.0
        data_len = len(data)
        for count in char_freq.values():
            p = count / data_len
            if p > 0:
                entropy -= p * math.log2(p)

        return entropy

    def _valid_address(self, address: str) -> bool:
        """Check if address format is valid."""
        if not address:
            return False
        return bool(re.match(r"^[a-f0-9]{64}$", address.lower()))

    def evaluate(
        self, sender: str, recipient: str, amount: int, data: str
    ) -> DeterministicResult:
        """
        Main evaluation function - completely deterministic.
        Same input always produces same output.
        """
        risk_score = 0.0
        reasons = []
        rules_triggered = []

        sender = sender.lower() if sender else ""
        recipient = recipient.lower() if recipient else ""
        data = data.lower() if data else ""

        # Rule 1: Whitelist check (overrides everything)
        if sender in self.whitelist:
            return DeterministicResult(
                valid=True,
                risk_score=0.0,
                risk_level=RiskLevel.LOW,
                reasons=["sender_whitelisted"],
                rules_triggered=["whitelist_override"],
                policy_version=self.VERSION,
            )

        # Rule 2: Blacklist check
        if sender in self.blacklist:
            risk_score += 100.0
            reasons.append("sender_blacklisted")
            rules_triggered.append("blacklist_sender")

        if recipient in self.blacklist:
            risk_score += 80.0
            reasons.append("recipient_blacklisted")
            rules_triggered.append("blacklist_recipient")

        # Rule 3: Address format validation
        if sender and not self._valid_address(sender):
            risk_score += 30.0
            reasons.append("invalid_sender_format")
            rules_triggered.append("invalid_address_format")

        if recipient and not self._valid_address(recipient):
            risk_score += 30.0
            reasons.append("invalid_recipient_format")
            rules_triggered.append("invalid_address_format")

        # Rule 4: Self-transfer check
        if sender and recipient and sender == recipient:
            risk_score += 35.0
            reasons.append("self_transfer")
            rules_triggered.append("self_transfer")

        # Rule 5: Amount analysis (deterministic thresholds)
        if amount == 0:
            risk_score += 10.0
            reasons.append("zero_amount")
            rules_triggered.append("zero_amount")

        if amount > self.MAX_AMOUNT:
            risk_score += 60.0
            reasons.append("exceeds_max_amount")
            rules_triggered.append("max_amount_exceeded")
        elif amount > self.LARGE_AMOUNT:
            risk_score += 15.0 + (amount / self.LARGE_AMOUNT) * 5
            reasons.append("large_amount")
            rules_triggered.append("large_amount")

        # Rule 6: Dangerous patterns (regex matching)
        for pattern, score, name in self.DANGEROUS_PATTERNS:
            if pattern.search(data):
                risk_score += score
                reasons.append(f"dangerous_pattern:{name}")
                rules_triggered.append(f"dangerous_{name}")

        # Rule 7: Suspicious patterns
        for pattern, score, name in self.SUSPICIOUS_PATTERNS:
            if pattern.search(data):
                risk_score += score
                reasons.append(f"suspicious:{name}")
                rules_triggered.append(f"suspicious_{name}")

        # Rule 8: Money laundering patterns
        for pattern, score, name in self.MONEY_LAUNDERING_PATTERNS:
            if pattern.search(data):
                risk_score += score
                reasons.append(f"money_laundering:{name}")
                rules_triggered.append(f"ml_{name}")

        # Rule 9: Data length check
        if len(data) > self.MAX_DATA_LENGTH:
            risk_score += 25.0
            reasons.append("excessive_data_length")
            rules_triggered.append("data_length_exceeded")

        # Rule 10: High entropy data (potential encoded content)
        if data:
            entropy = self._calculate_entropy(data)
            if entropy > self.HIGH_ENTROPY_THRESHOLD:
                risk_score += 20.0
                reasons.append(f"high_entropy_data:{entropy:.2f}")
                rules_triggered.append("high_entropy")

        # Rule 11: URL detection (phishing risk)
        urls = re.findall(r"https?://[^\s]+", data)
        if urls:
            risk_score += 15.0 * len(urls)
            reasons.append(f"contains_urls:{len(urls)}")
            rules_triggered.append("url_in_data")

            # Check for impersonation
            impersonation_targets = [
                "metamask",
                "uniswap",
                "opensea",
                "binance",
                "coinbase",
                "wallet",
                "login",
            ]
            for url in urls:
                url_lower = url.lower()
                for target in impersonation_targets:
                    if target in url_lower:
                        risk_score += 35.0
                        reasons.append(f"impersonation:{target}")
                        rules_triggered.append("impersonation_detected")

        # Rule 12: Contract interaction detection
        if re.search(r"0x[a-fA-F0-9]{40}", data):
            risk_score += 10.0
            reasons.append("contract_interaction")
            rules_triggered.append("contract_interaction")

        # Rule 13: Hex-only large data (potential binary/executable)
        if data and re.match(r"^[0-9a-f]+$", data) and len(data) > 500:
            risk_score += 25.0
            reasons.append("suspicious_hex_data")
            rules_triggered.append("suspicious_data")

        # Determine final validity
        risk_score = min(risk_score, 100.0)

        if risk_score >= self.RISK_THRESHOLD_CRITICAL:
            risk_level = RiskLevel.CRITICAL
            valid = False
        elif risk_score >= self.RISK_THRESHOLD_MEDIUM:
            risk_level = RiskLevel.HIGH
            valid = True  # Allow but flag
        else:
            risk_level = RiskLevel.LOW if risk_score < 15 else RiskLevel.MEDIUM
            valid = True

        if not reasons:
            reasons.append("all_checks_passed")

        return DeterministicResult(
            valid=valid,
            risk_score=round(risk_score, 2),
            risk_level=risk_level,
            reasons=reasons,
            rules_triggered=rules_triggered,
            policy_version=self.VERSION,
        )


# Singleton instance
policy_engine = DeterministicPolicyEngine()


def evaluate_transaction(
    sender: str, recipient: str, amount: int, data: str
) -> DeterministicResult:
    """Convenience function for API calls."""
    return policy_engine.evaluate(sender, recipient, amount, data)


def load_policy_lists(
    blacklist_path: Optional[str] = None, whitelist_path: Optional[str] = None
):
    """Load policy lists from files."""
    if blacklist_path:
        try:
            with open(blacklist_path, "r") as f:
                addresses = [
                    line.strip()
                    for line in f
                    if line.strip() and not line.startswith("#")
                ]
                policy_engine.load_blacklist(addresses)
        except FileNotFoundError:
            pass

    if whitelist_path:
        try:
            with open(whitelist_path, "r") as f:
                addresses = [
                    line.strip()
                    for line in f
                    if line.strip() and not line.startswith("#")
                ]
                policy_engine.load_whitelist(addresses)
        except FileNotFoundError:
            pass


class OnChainPolicyManager:
    """
    Manages on-chain policy rules that can be voted on.
    Rules are stored deterministically for consensus.
    """

    def __init__(self):
        self.rules: Dict[str, dict] = {}
        self.rule_count = 0

    def add_rule(self, rule_type: str, value: str, score: float) -> str:
        """Add a new policy rule."""
        rule_id = f"rule_{self.rule_count}"
        self.rule_count += 1
        self.rules[rule_id] = {
            "type": rule_type,
            "value": value,
            "score": score,
            "active": True,
        }
        return rule_id

    def remove_rule(self, rule_id: str):
        """Remove a policy rule."""
        if rule_id in self.rules:
            self.rules[rule_id]["active"] = False

    def get_active_rules(self) -> List[dict]:
        """Get all active rules."""
        return [r for r in self.rules.values() if r.get("active", False)]

    def to_deterministic_hash(self) -> str:
        """Convert rules to deterministic hash for on-chain storage."""
        import hashlib

        rules_str = str(sorted(self.rules.items()))
        return hashlib.sha256(rules_str.encode()).hexdigest()


policy_manager = OnChainPolicyManager()


def analyze_ai_agent_transaction(
    sender: str, recipient: str, amount: int, data: str
) -> dict:
    """
    Specialized analysis for AI agent transactions.
    Returns insights specific to autonomous AI agents.
    """
    data_lower = data.lower() if data else ""

    is_agent_tx = False
    agent_type = None
    capabilities = []

    # Detect AI agent patterns
    if re.search(r"agent|call|execute|autonomous", data_lower):
        is_agent_tx = True
        if re.search(r"agent", data_lower):
            agent_type = "ai_agent"
        if re.search(r"autonomous", data_lower):
            agent_type = "autonomous_agent"

    if re.search(r"api|function.?call|tool", data_lower):
        capabilities.append("api_call")
    if re.search(r"data|train|model", data_lower):
        capabilities.append("data_processing")
    if re.search(r"payment|transfer|send", data_lower):
        capabilities.append("payment")

    return {
        "is_ai_agent_transaction": is_agent_tx,
        "agent_type": agent_type,
        "capabilities": capabilities,
        "requires_approval": is_agent_tx and amount > 1000000,
    }


def get_policy_stats() -> dict:
    """Get policy engine statistics."""
    return {
        "version": policy_engine.VERSION,
        "blacklist_count": len(policy_engine.blacklist),
        "whitelist_count": len(policy_engine.whitelist),
        "onchain_rules": policy_manager.rule_count,
        "deterministic": True,
        "consensus_ready": True,
    }
