// NogoChain Web Wallet - Proposals Module
// Full feature parity with /proposals/ page

let allProposals = [];
let currentFilter = 'all';

// API helper
async function proposalsApi(url) {
    try {
        const response = await fetch(url);
        if (!response.ok) throw new Error('HTTP ' + response.status);
        return await response.json();
    } catch (e) {
        console.error('API error:', e);
        proposalsToast('API error: ' + e.message, 'error');
        return null;
    }
}

// Load proposals
async function loadProposals() {
    const proposals = await proposalsApi('/api/proposals');
    if (!proposals) {
        document.getElementById('proposalsList').innerHTML = '<div class="empty-state"><h3>Failed to load proposals</h3><p>Please check your connection</p></div>';
        return;
    }
    
    allProposals = proposals || [];
    allProposals.sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0));
    
    updateStats();
    renderProposals();
}

// Handle vote
async function handleVote(proposalId, support) {
    const voterAddress = document.getElementById('voterAddress').value;
    const votingPower = document.getElementById('votingPower').value;
    
    if (!voterAddress) {
        proposalsToast('Please enter your NOGO address', 'error');
        return;
    }
    
    if (!votingPower || votingPower < 1) {
        proposalsToast('Voting power must be at least 1', 'error');
        return;
    }
    
    const voteData = {
        proposalId: proposalId,
        voter: voterAddress,
        support: support,
        votingPower: Math.floor(parseFloat(votingPower) * 1e8)
    };
    
    console.log('Submitting vote:', voteData);
    
    try {
        const response = await fetch('/api/proposals/vote', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(voteData)
        });
        
        const result = await response.json();
        console.log('Vote response:', result);
        
        if (response.ok && result.success) {
            proposalsToast('Vote submitted successfully!', 'success');
            loadProposals();
            showProposalDetail(proposalId);
        } else {
            const errorMsg = result.error || 'Failed to submit vote';
            console.error('Vote error:', errorMsg);
            proposalsToast('Error: ' + errorMsg, 'error');
        }
    } catch (error) {
        console.error('Network error:', error);
        proposalsToast('Network error: ' + error.message, 'error');
    }
}

// Update statistics
function updateStats() {
    const total = allProposals.length;
    const statusMap = {0: 'active', 1: 'passed', 2: 'rejected', 3: 'executed', 4: 'expired'};
    
    const active = allProposals.filter(p => statusMap[p.status] === 'active').length;
    const passed = allProposals.filter(p => statusMap[p.status] === 'passed').length;
    const rejected = allProposals.filter(p => statusMap[p.status] === 'rejected').length;
    const executed = allProposals.filter(p => statusMap[p.status] === 'executed').length;
    
    document.getElementById('totalProposals').textContent = total;
    document.getElementById('activeProposals').textContent = active;
    document.getElementById('passedProposals').textContent = passed;
    document.getElementById('rejectedProposals').textContent = rejected;
    document.getElementById('executedProposals').textContent = executed;
}

// Filter proposals
function filterProposals(filter) {
    currentFilter = filter;
    renderProposals();
    
    // Update tab active state
    document.querySelectorAll('.filter-tab').forEach(tab => {
        tab.classList.remove('active');
    });
    event.target.classList.add('active');
}

// Render proposals
function renderProposals() {
    const container = document.getElementById('proposalsList');
    
    let filtered = allProposals;
    if (currentFilter !== 'all') {
        const statusMap = {0: 'active', 1: 'passed', 2: 'rejected', 3: 'executed', 4: 'expired'};
        filtered = allProposals.filter(p => statusMap[p.status] === currentFilter);
    }
    
    if (filtered.length === 0) {
        container.innerHTML = `
            <div class="empty-state">
                <h3>No ${currentFilter !== 'all' ? currentFilter : ''} proposals</h3>
                <p>Proposals matching your filter will appear here</p>
            </div>
        `;
        return;
    }
    
    container.innerHTML = filtered.map(p => {
        const totalVotes = (p.votesFor || 0) + (p.votesAgainst || 0);
        const approvalPercent = totalVotes > 0 ? ((p.votesFor || 0) * 100 / totalVotes).toFixed(1) : 0;
        
        // Convert type number to string
        const typeMap = {0: 'treasury', 1: 'ecosystem', 2: 'grant', 3: 'event'};
        const typeStr = typeMap[p.type] || p.type || 'unknown';
        
        // Convert status number to string
        const statusMap = {0: 'active', 1: 'passed', 2: 'rejected', 3: 'executed', 4: 'expired'};
        const statusStr = statusMap[p.status] || p.status || 'unknown';
        
        console.log('Proposal:', p);
        console.log('Proposal ID:', p.id);
        console.log('Status (raw):', p.status, '-> Status (converted):', statusStr);
        
        return `
            <div class="proposal-card" onclick="showProposalDetail('${p.id || ''}')">
                <div class="proposal-header">
                    <div class="proposal-title">${escapeHtml(p.title)}</div>
                    <div class="proposal-status status-${statusStr}">${statusStr}</div>
                </div>
                
                <div style="margin-bottom: 15px;">
                    <div style="font-size: 11px; color: var(--text-secondary); margin-bottom: 3px;">Proposal ID</div>
                    <div style="font-family: monospace; font-size: 11px; color: var(--accent-blue); word-break: break-all;">${p.id || '-'}</div>
                </div>
                
                <div class="proposal-meta">
                    <div class="meta-item">
                        <div class="meta-label">Type</div>
                        <div class="meta-value">${typeStr}</div>
                    </div>
                    <div class="meta-item">
                        <div class="meta-label">Amount</div>
                        <div class="meta-value">${formatAmount(p.amount)} NOGO</div>
                    </div>
                    <div class="meta-item">
                        <div class="meta-label">Created</div>
                        <div class="meta-value">${formatDate(p.createdAt)}</div>
                    </div>
                    <div class="meta-item">
                        <div class="meta-label">Voting Ends</div>
                        <div class="meta-value">${formatDate(p.votingEndTime)}</div>
                    </div>
                </div>
                
                <div class="proposal-description">${escapeHtml(p.description)}</div>
                
                <div class="voting-section">
                    <div class="voting-stats">
                        <div class="vote-item">
                            <span class="vote-icon">👍</span>
                            <span class="vote-number">${(p.votesFor || 0) / 1e8}</span>
                        </div>
                        <div class="vote-item">
                            <span class="vote-icon">👎</span>
                            <span class="vote-number">${(p.votesAgainst || 0) / 1e8}</span>
                        </div>
                        <div class="vote-item">
                            <span class="vote-icon">📊</span>
                            <span class="vote-number">${approvalPercent}% Approval</span>
                        </div>
                    </div>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${approvalPercent}%"></div>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

// Show proposal detail
async function showProposalDetail(proposalId) {
    const proposal = await proposalsApi('/api/proposals/' + proposalId);
    if (!proposal) return;
    
    // Convert status number to string
    const statusMap = {0: 'active', 1: 'passed', 2: 'rejected', 3: 'executed', 4: 'expired'};
    const statusStr = statusMap[proposal.status] || proposal.status || 'unknown';
    
    const container = document.getElementById('proposalsList');
    container.innerHTML = `
        <div style="display: flex; align-items: center; gap: 15px; margin-bottom: 20px;">
            <button onclick="loadProposals()" style="padding: 10px 20px; background: var(--accent-blue); color: white; border: none; border-radius: 6px; cursor: pointer; font-weight: 600; font-size: 14px; display: flex; align-items: center; gap: 6px; transition: all 0.2s;">← Back to Proposals</button>
        </div>
        <div class="proposal-detail">
            <div class="detail-section">
                <div class="detail-label">Proposal ID</div>
                <div class="detail-value" style="font-family: monospace; font-size: 12px; word-break: break-all;">${proposal.id}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Title</div>
                <div class="detail-value">${escapeHtml(proposal.title)}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Status</div>
                <div class="detail-value">
                    <span class="proposal-status status-${statusStr}">${statusStr}</span>
                </div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Description</div>
                <div class="detail-value">${escapeHtml(proposal.description)}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Type</div>
                <div class="detail-value">${proposal.type || 'unknown'}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Amount</div>
                <div class="detail-value">${formatAmount(proposal.amount)} NOGO</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Recipient</div>
                <div class="detail-value">${proposal.recipient || '-'}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Proposer</div>
                <div class="detail-value">${proposal.proposer || '-'}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Created At</div>
                <div class="detail-value">${formatDate(proposal.createdAt)}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Voting End Time</div>
                <div class="detail-value">${formatDate(proposal.votingEndTime)}</div>
            </div>
            
            <div class="detail-section">
                <div class="detail-label">Voting Results</div>
                <div class="detail-value">
                    <div style="margin-bottom: 10px;">
                        👍 For: <strong>${(proposal.votesFor || 0) / 1e8}</strong>
                    </div>
                    <div style="margin-bottom: 10px;">
                        👎 Against: <strong>${(proposal.votesAgainst || 0) / 1e8}</strong>
                    </div>
                    <div>
                        Total Votes: <strong>${((proposal.votesFor || 0) + (proposal.votesAgainst || 0)) / 1e8}</strong>
                    </div>
                </div>
            </div>
            
            ${statusStr === 'active' ? `
            <div class="detail-section" style="margin-top: 30px; padding-top: 30px; border-top: 1px solid var(--border-color);">
                <div class="detail-label">Cast Your Vote</div>
                <div style="margin-top: 15px; display: flex; gap: 15px;">
                    <button onclick="handleVote('${proposal.id}', true)" style="flex: 1; padding: 12px; background: var(--accent-green); color: white; border: none; border-radius: 6px; font-weight: 600; cursor: pointer; font-size: 14px;">👍 Vote For</button>
                    <button onclick="handleVote('${proposal.id}', false)" style="flex: 1; padding: 12px; background: var(--accent-red); color: white; border: none; border-radius: 6px; font-weight: 600; cursor: pointer; font-size: 14px;">👎 Vote Against</button>
                </div>
                <div style="margin-top: 15px;">
                    <input type="text" id="voterAddress" placeholder="Your NOGO Address" style="width: 100%; padding: 10px; background: var(--bg-tertiary); border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-primary); font-size: 14px;" />
                </div>
                <div style="margin-top: 10px;">
                    <input type="number" id="votingPower" placeholder="Voting Power (NOGO balance)" value="1" min="1" style="width: 100%; padding: 10px; background: var(--bg-tertiary); border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-primary); font-size: 14px;" />
                </div>
                <div style="margin-top: 10px; font-size: 12px; color: var(--text-secondary);">
                    ⚠️ Test Mode: No actual voting power verification
                </div>
            </div>
            ` : ''}
        </div>
    `;
}

// Helper functions
function formatAmount(wei) {
    return ((wei || 0) / 1e8).toFixed(8);
}

function formatDate(timestamp) {
    if (!timestamp) return '-';
    return new Date(timestamp * 1000).toLocaleString();
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Modal functions
function showCreateModal() {
    document.getElementById('createModal').style.display = 'flex';
}

function closeModal() {
    document.getElementById('createModal').style.display = 'none';
    document.getElementById('createProposalForm').reset();
}

// Close modal when clicking outside
if (document.getElementById('createModal')) {
    document.getElementById('createModal').addEventListener('click', function(e) {
        if (e.target === this) {
            closeModal();
        }
    });
}

// Handle proposal creation
async function handleSubmit(event) {
    event.preventDefault();
    
    const formData = {
        proposer: document.getElementById('proposer').value,
        title: document.getElementById('title').value,
        description: document.getElementById('description').value,
        type: document.getElementById('type').value,
        amount: Math.floor(parseFloat(document.getElementById('amount').value) * 1e8),
        recipient: document.getElementById('recipient').value,
        deposit: Math.floor(parseFloat(document.getElementById('deposit').value) * 1e8)
    };
    
    try {
        const response = await fetch('/api/proposals/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(formData)
        });
        
        const result = await response.json();
        
        if (response.ok && result.success) {
            proposalsToast('Proposal created successfully! ID: ' + result.proposalId, 'success');
            closeModal();
            loadProposals();
        } else {
            proposalsToast('Error: ' + (result.error || 'Failed to create proposal'), 'error');
        }
    } catch (error) {
        proposalsToast('Network error: ' + error.message, 'error');
    }
}

// Toast notification
function proposalsToast(message, type = 'info') {
    const toast = document.getElementById('proposalsToast');
    const toastMessage = document.getElementById('toastMessage');
    
    // Set color based on type
    if (type === 'success') {
        toast.style.background = 'var(--accent-green)';
    } else if (type === 'error') {
        toast.style.background = 'var(--accent-red)';
    } else {
        toast.style.background = 'var(--accent-blue)';
    }
    
    toastMessage.textContent = message;
    toast.style.display = 'block';
    toast.style.animation = 'slideIn 0.3s ease-out';
    
    setTimeout(() => {
        toast.style.animation = 'slideOut 0.3s ease-out';
        setTimeout(() => {
            toast.style.display = 'none';
        }, 300);
    }, 3000);
}

// Navigation functions
function showProposalsSection() {
    document.getElementById('welcomeSection').style.display = 'none';
    document.getElementById('walletDashboardTab').style.display = 'none';
    document.getElementById('proposalsSection').style.display = 'block';
    loadProposals();
}

function showWalletSection() {
    document.getElementById('proposalsSection').style.display = 'none';
    document.getElementById('welcomeSection').style.display = 'block';
    document.getElementById('walletDashboardTab').style.display = 'block';
}

console.log('Proposals module loaded');
