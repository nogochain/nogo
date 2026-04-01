#!/usr/bin/env node

const axios = require('axios');

class NogoChainRPC {
  constructor(baseURL = 'http://localhost:8080') {
    this.client = axios.create({
      baseURL,
      timeout: 30000,
      headers: { 'Content-Type': 'application/json' }
    });
  }

  async getChainInfo() {
    const { data } = await this.client.get('/chain/info');
    return data;
  }

  async getBalance(address) {
    const { data } = await this.client.get(`/balance/${address}`);
    return data;
  }

  async getBlock(heightOrHash) {
    const endpoint = isNaN(heightOrHash) ? `/block/hash/${heightOrHash}` : `/block/height/${heightOrHash}`;
    const { data } = await this.client.get(endpoint);
    return data;
  }

  async getTransaction(txId) {
    const { data } = await this.client.get(`/tx/${txId}`);
    return data;
  }

  async getMempool() {
    const { data } = await this.client.get('/mempool');
    return data;
  }

  async getAddressTransactions(address, limit = 50) {
    const { data } = await this.client.get(`/address/${address}/txs?limit=${limit}`);
    return data;
  }

  async createWallet() {
    const { data } = await this.client.post('/wallet/create');
    return data;
  }

  async sendTransaction(tx) {
    const { data } = await this.client.post('/tx', tx);
    return data;
  }

  async auditTransaction(tx) {
    const { data } = await this.client.post('/audit', { transaction: tx });
    return data;
  }

  async getTxProof(txId) {
    const { data } = await this.client.get(`/tx/proof/${txId}`);
    return data;
  }

  subscribeToMempool(callback) {
    const ws = new WebSocket(this.client.defaults.baseURL.replace('http', 'ws') + '/ws');
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'mempool') {
        callback(msg.data);
      }
    };
    return ws;
  }

  subscribeToBlocks(callback) {
    const ws = new WebSocket(this.client.defaults.baseURL.replace('http', 'ws') + '/ws');
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      if (msg.type === 'block') {
        callback(msg.data);
      }
    };
    return ws;
  }
}

module.exports = { NogoChainRPC };
module.exports.default = NogoChainRPC;