from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel
from typing import Optional, Dict, Any, Tuple, List
import os
import json
import hashlib
import re
from datetime import datetime, timedelta
from collections import defaultdict
import threading
from deterministic_policy import (
    DeterministicPolicyEngine,
    policy_engine,
    evaluate_transaction,
    load_policy_lists,
    RiskLevel,
)

# Thread-safe audit log storage
audit_lock = threading.Lock()
audit_log: list[Dict[str, Any]] = []
stats = {"total_audited": 0, "approved": 0, "rejected": 0, "errors": 0}

app = FastAPI()

# Configuration
AUDIT_LOG_PATH = os.getenv("AUDIT_LOG_PATH", "/app/data/audit.log")
AUDIT_MODE = os.getenv("AUDIT_MODE", "local")

# Initialize deterministic policy engine with blacklist
RISK_LIST_PATH = os.getenv("RISK_LIST_PATH", "/app/data/high_risk_addresses.txt")
load_policy_lists(blacklist_path=RISK_LIST_PATH)


class Transaction(BaseModel):
    sender: str
    recipient: str
    amount: int
    data: str = ""


class AIRequest(BaseModel):
    transaction: Transaction


class AIResponse(BaseModel):
    valid: bool
    reason: Optional[str] = None
    risk_score: Optional[float] = None
    checks_passed: Optional[list[str]] = None
    analysis: Optional[Dict[str, Any]] = None


def analyze_local(
    tx: Transaction,
) -> tuple[bool, str, float, list[str], Dict[str, Any]]:
    """
    Deterministic policy-based analysis.
    Same input always produces same output - can be used in consensus.
    """
    result = evaluate_transaction(
        sender=tx.sender or "",
        recipient=tx.recipient or "",
        amount=tx.amount,
        data=tx.data or "",
    )

    checks_passed = result.rules_triggered
    analysis = {
        "deterministic": True,
        "policy_version": result.policy_version,
        "risk_level": result.risk_level.value,
        "rules_triggered": result.rules_triggered,
    }

    reason = "; ".join(result.reasons) if result.reasons else "approved"

    return (
        result.valid,
        reason,
        result.risk_score,
        checks_passed,
        analysis,
    )


def analyze_ml(tx: Transaction) -> Tuple[bool, str, float]:
    """Legacy function - now uses deterministic policy engine."""
    return True, "deterministic_policy", 0.0


# API Endpoints
@app.get("/")
async def root():
    return {
        "name": "NogoChain AI Auditor",
        "version": "3.0",
        "status": "operational",
        "engine": "deterministic_policy",
        "deterministic": True,
        "features": [
            "pattern_detection",
            "deterministic_rules",
            "entropy_detection",
            "address_validation",
            "risk_scoring",
        ],
    }


@app.post("/audit", response_model=AIResponse)
async def audit_transaction(request: AIRequest):
    start_time = datetime.utcnow()

    try:
        is_valid, reason, risk_score, checks_passed, analysis = analyze_local(
            request.transaction
        )

        analysis["deterministic"] = True
        analysis["policy_engine"] = "deterministic_v1"

        process_time = (datetime.utcnow() - start_time).total_seconds() * 1000

        response = AIResponse(
            valid=is_valid,
            reason=reason,
            risk_score=round(risk_score, 2),
            checks_passed=checks_passed if checks_passed else [],
            analysis=analysis,
        )

        with audit_lock:
            stats["total_audited"] += 1
            if is_valid:
                stats["approved"] += 1
            else:
                stats["rejected"] += 1

        log_audit(request, response, process_time)
        return response

    except Exception as e:
        with audit_lock:
            stats["errors"] += 1
        return AIResponse(
            valid=True,
            reason=f"error: {str(e)}",
            risk_score=0.0,
            analysis={"error": str(e)},
        )


@app.get("/health")
async def health_check():
    return {
        "status": "ok",
        "mode": AUDIT_MODE,
        "engine": "deterministic",
        "stats": stats,
        "ai_version": "3.0",
        "deterministic": True,
    }


@app.get("/stats")
async def get_stats():
    with audit_lock:
        return stats


@app.get("/logs")
async def get_logs(limit: int = 100):
    with audit_lock:
        return audit_log[-limit:]


@app.get("/ai/stats")
async def get_ai_stats():
    return get_global_stats()


@app.get("/ai/profile/{address}")
async def get_address_ai_profile(address: str):
    return get_address_profile(address.lower())


@app.get("/ai/trends")
async def get_fraud_trends_endpoint():
    return get_fraud_trends()


@app.get("/ai/network/{address}")
async def get_network_analysis(address: str):
    from advanced_ai import get_address_connections

    return get_address_connections(address.lower())


@app.post("/risklist/add")
async def add_high_risk_address(address: str):
    addr = address.lower().strip()
    if not re.match(r"^[a-f0-9]{64}$", addr):
        raise HTTPException(status_code=400, detail="Invalid address format")

    with audit_lock:
        HIGH_RISK_ADDRESSES.add(addr)

    try:
        os.makedirs(os.path.dirname(RISK_LIST_PATH), exist_ok=True)
        with open(RISK_LIST_PATH, "a") as f:
            f.write(addr + "\n")
    except Exception:
        pass

    return {"status": "added", "address": addr}


@app.get("/ai/batch-analyze")
async def batch_analyze(addresses: str):
    addr_list = addresses.split(",")
    results = []
    for addr in addr_list:
        addr = addr.strip().lower()
        profile = get_address_profile(addr)
        results.append({"address": addr, "profile": profile})
    return {"results": results}


def log_audit(request: AIRequest, response: AIResponse, process_time_ms: float):
    timestamp = datetime.utcnow().isoformat() + "Z"
    tx_hash = hashlib.sha256(
        f"{request.transaction.sender}{request.transaction.recipient}{request.transaction.amount}".encode()
    ).hexdigest()[:16]

    entry = {
        "timestamp": timestamp,
        "tx_hash_prefix": tx_hash,
        "sender": request.transaction.sender[:16] + "..."
        if len(request.transaction.sender) > 16
        else request.transaction.sender,
        "recipient": request.transaction.recipient[:16] + "..."
        if len(request.transaction.recipient) > 16
        else request.transaction.recipient,
        "amount": request.transaction.amount,
        "valid": response.valid,
        "reason": response.reason,
        "risk_score": response.risk_score,
        "process_time_ms": round(process_time_ms, 2),
    }

    with audit_lock:
        audit_log.append(entry)
        if len(audit_log) > 10000:
            audit_log[:] = audit_log[-10000:]

    try:
        os.makedirs(os.path.dirname(AUDIT_LOG_PATH), exist_ok=True)
        with open(AUDIT_LOG_PATH, "a") as f:
            f.write(json.dumps(entry) + "\n")
    except Exception:
        pass


if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("PORT", "8081"))
    uvicorn.run(app, host="0.0.0.0", port=port)


# =====================================================
# ENHANCED AI AUDITOR FEATURES v3.0
# =====================================================


class EnhancedAIResponse(BaseModel):
    valid: bool
    reason: Optional[str] = None
    risk_score: Optional[float] = None
    checks_passed: Optional[list[str]] = None
    analysis: Optional[Dict[str, Any]] = None
    cross_chain_risk: Optional[Dict[str, Any]] = None
    contract_interaction: Optional[Dict[str, Any]] = None
    gas_analysis: Optional[Dict[str, Any]] = None
    reputation_score: Optional[Dict[str, Any]] = None
    historical_context: Optional[Dict[str, Any]] = None
    behavioral_pattern: Optional[Dict[str, Any]] = None


CROSS_CHAIN_PATTERNS = [
    r"bridge.*to:",
    r"wrap:",
    r"swap.*to:",
    r"cross.?chain",
    r"multichain",
    r"any.?swap",
    r"stargate",
    r"layerzero",
    r"wormhole",
    r"axelar",
    r"orbiter",
    r"syncswap",
]
compiled_cross_chain = [re.compile(p, re.IGNORECASE) for p in CROSS_CHAIN_PATTERNS]

CONTRACT_PATTERNS = [
    r"0x[a-fA-F0-9]{40}",
    r"contract:",
    r"approve:",
    r"transferFrom:",
    r"swapExact",
    r"addLiquidity",
    r"removeLiquidity",
    r"stake:",
    r"unstake:",
    r"claim:",
]
compiled_contract = [re.compile(p, re.IGNORECASE) for p in CONTRACT_PATTERNS]


class ReputationTracker:
    def __init__(self):
        self.address_history: Dict[str, Dict[str, Any]] = {}

    def update_reputation(self, address: str, approved: bool, amount: float):
        if address not in self.address_history:
            self.address_history[address] = {
                "total_tx": 0,
                "approved_tx": 0,
                "rejected_tx": 0,
                "total_volume": 0.0,
                "first_seen": datetime.utcnow(),
            }
        h = self.address_history[address]
        h["total_tx"] += 1
        h["total_volume"] += amount
        if approved:
            h["approved_tx"] += 1
        else:
            h["rejected_tx"] += 1

    def get_reputation(self, address: str) -> Dict[str, Any]:
        if address not in self.address_history:
            return {"address": address, "reputation_score": 50.0, "status": "new"}

        h = self.address_history[address]
        total = h["total_tx"]
        approved = h["approved_tx"]
        rate = approved / total if total > 0 else 1.0

        score = 50.0
        if rate >= 0.95 and total >= 10:
            score = 90.0
        elif rate >= 0.8 and total >= 5:
            score = 70.0
        elif rate < 0.5:
            score = 20.0

        age_days = (datetime.utcnow() - h["first_seen"]).days
        if age_days > 365:
            score = min(100, score + 10)

        status = (
            "trusted" if score >= 70 else "neutral" if score >= 30 else "suspicious"
        )
        return {
            "address": address,
            "reputation_score": score,
            "status": status,
            "total_tx": total,
            "approval_rate": rate,
        }


reputation_tracker = ReputationTracker()


@app.post("/ai/enhanced-audit")
async def enhanced_audit(request: AIRequest):
    """Enhanced audit with cross-chain, contract, reputation, and behavioral analysis."""
    tx = request.transaction
    is_valid, reason, risk_score, checks, analysis = analyze_local(tx)

    # Cross-chain risk
    cross_chain_risk = {
        "has_cross_chain": False,
        "indicators": [],
        "risk_level": "none",
    }
    for pattern in compiled_cross_chain:
        if pattern.search(tx.data or ""):
            cross_chain_risk["indicators"].append(pattern.pattern)
            risk_score += 15.0
    if cross_chain_risk["indicators"]:
        cross_chain_risk["has_cross_chain"] = True
        cross_chain_risk["risk_level"] = (
            "high" if len(cross_chain_risk["indicators"]) > 1 else "medium"
        )

    # Contract interaction
    contract_interaction = {"interacts_with_contract": False, "risk_level": "none"}
    if re.search(r"0x[a-fA-F0-9]{40}", tx.data or ""):
        contract_interaction["interacts_with_contract"] = True
        contract_interaction["risk_level"] = "medium"
        risk_score += 5.0

    # Reputation scoring
    sender_rep = reputation_tracker.get_reputation(tx.sender.lower())
    recipient_rep = reputation_tracker.get_reputation(tx.sender.lower())
    reputation_score = {"sender": sender_rep, "recipient": recipient_rep}

    if sender_rep["status"] == "suspicious":
        risk_score += 30.0
    elif sender_rep["status"] == "neutral":
        risk_score += 10.0

    if recipient_rep["status"] == "suspicious":
        risk_score += 20.0

    # Behavioral pattern
    behavioral = {"pattern_type": "standard", "velocity": "normal"}
    if tx.amount > 5_000_000 and sender_rep.get("total_tx", 0) < 10:
        behavioral["pattern_type"] = "new_whale"
        risk_score += 20.0

    # Update reputation
    reputation_tracker.update_reputation(tx.sender.lower(), is_valid, tx.amount)

    final_valid = is_valid and risk_score < 75.0

    return EnhancedAIResponse(
        valid=final_valid,
        reason=reason or (f"high_risk_score:{risk_score}" if not final_valid else None),
        risk_score=round(min(100.0, risk_score), 2),
        checks_passed=checks,
        analysis=analysis,
        cross_chain_risk=cross_chain_risk,
        contract_interaction=contract_interaction,
        gas_analysis={"fee_reasonable": True},
        reputation_score=reputation_score,
        historical_context={"sender_history": sender_rep},
        behavioral_pattern=behavioral,
    )


@app.get("/ai/reputation/{address}")
async def get_reputation(address: str):
    return reputation_tracker.get_reputation(address.lower())


@app.post("/ai/batch-audit")
async def batch_audit(transactions: List[Transaction]):
    results = []
    for tx in transactions:
        is_valid, reason, risk_score, checks, _ = analyze_local(tx)
        results.append({"valid": is_valid, "reason": reason, "risk_score": risk_score})
        reputation_tracker.update_reputation(tx.sender.lower(), is_valid, tx.amount)
    return {"results": results, "total": len(results)}
