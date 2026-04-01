#!/usr/bin/env python3

import requests
import json
from typing import Optional, Dict, Any, List


class NogoChainRPC:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({"Content-Type": "application/json"})

    def get_chain_info(self) -> Dict[str, Any]:
        return self.session.get(f"{self.base_url}/chain/info").json()

    def get_balance(self, address: str) -> Dict[str, Any]:
        return self.session.get(f"{self.base_url}/balance/{address}").json()

    def get_block(self, height_or_hash: int | str) -> Dict[str, Any]:
        endpoint = (
            f"/block/hash/{height_or_hash}"
            if isinstance(height_or_hash, str)
            else f"/block/height/{height_or_hash}"
        )
        return self.session.get(f"{self.base_url}{endpoint}").json()

    def get_transaction(self, tx_id: str) -> Dict[str, Any]:
        return self.session.get(f"{self.base_url}/tx/{tx_id}").json()

    def get_mempool(self) -> List[Dict[str, Any]]:
        return self.session.get(f"{self.base_url}/mempool").json()

    def get_address_txs(self, address: str, limit: int = 50) -> List[Dict[str, Any]]:
        return self.session.get(
            f"{self.base_url}/address/{address}/txs?limit={limit}"
        ).json()

    def create_wallet(self) -> Dict[str, Any]:
        return self.session.post(f"{self.base_url}/wallet/create").json()

    def send_transaction(self, tx: Dict[str, Any]) -> Dict[str, Any]:
        return self.session.post(f"{self.base_url}/tx", json=tx).json()

    def audit_transaction(self, tx: Dict[str, Any]) -> Dict[str, Any]:
        return self.session.post(
            f"{self.base_url}/audit", json={"transaction": tx}
        ).json()

    def get_tx_proof(self, tx_id: str) -> Dict[str, Any]:
        return self.session.get(f"{self.base_url}/tx/proof/{tx_id}").json()

    def subscribe_ws(self, callback):
        import websocket

        ws_url = self.base_url.replace("http", "ws") + "/ws"
        ws = websocket.WebSocketApp(ws_url, on_message=callback)
        ws.run_forever()
        return ws


if __name__ == "__main__":
    rpc = NogoChainRPC()
    print(rpc.get_chain_info())
