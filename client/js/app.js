// ---- API WRAPPER ----
const API_BASE = 'http://localhost:8080/api/v1';
let authToken = '';

function setToken(token) { authToken = token; }
function getToken() { return authToken; }

async function apiRequest(path, options = {}) {
    const headers = { 'Content-Type': 'application/json', ...options.headers };
    if (authToken) headers['Authorization'] = `Bearer ${authToken}`;

    const response = await fetch(`${API_BASE}${path}`, { ...options, headers });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `HTTP Error ${response.status}`);
    return data;
}

const api = {
    auth: {
        login: (username, password) => apiRequest('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
        register: (username, password, public_key) => apiRequest('/auth/register', { method: 'POST', body: JSON.stringify({ username, password, public_key }) })
    },
    users: {
        updatePublicKey: (public_key) => apiRequest('/users/me/public-key', { method: 'PUT', body: JSON.stringify({ public_key }) }),
        getMyKeys: () => apiRequest('/users/me/keys', { method: 'GET' }),
        getPublicKey: (userId) => apiRequest(`/users/${userId}/public-key`, { method: 'GET' }),
        getPresence: (userIds) => apiRequest(`/users/presence?ids=${userIds.join(',')}`, { method: 'GET' }),
        search: (query) => apiRequest(`/users/search?q=${encodeURIComponent(query)}`, { method: 'GET' })
    },
    conversations: {
        list: () => apiRequest('/conversations', { method: 'GET' }),
        createGroup: (name, member_ids) => apiRequest('/conversations', { method: 'POST', body: JSON.stringify({ name, member_ids }) }),
        getMessages: (conversationId, limit = 100, before_ts = null) => {
            let url = `/conversations/${conversationId}/messages?limit=${limit}`;
            if (before_ts) url += `&before_ts=${before_ts}`;
            return apiRequest(url, { method: 'GET' });
        }
    },
    chat: {
        sendMessage: (conversation_id, content_encrypted) => apiRequest('/chat/messages', { method: 'POST', body: JSON.stringify({ conversation_id, content_encrypted }) })
    },
    friendships: {
        request: (target_user_id) => apiRequest('/friendships/request', { method: 'POST', body: JSON.stringify({ target_user_id }) }),
        accept: (request_id) => apiRequest('/friendships/accept', { method: 'POST', body: JSON.stringify({ request_id }) }),
        reject: (request_id) => apiRequest('/friendships/reject', { method: 'POST', body: JSON.stringify({ request_id }) }),
        pending: () => apiRequest('/friendships/pending', { method: 'GET' })
    }
};

// ---- CRYPTO (JSEncrypt Wrapper) ----
const publicKeyCache = {};
const e2e = {
    storeKeys: (userId, publicPem, privatePem) => {
        localStorage.setItem(`e2ee_public_key_${userId}`, publicPem);
        localStorage.setItem(`e2ee_private_key_${userId}`, privatePem);
    },
    loadStoredKeys: (userId) => ({
        publicPem: localStorage.getItem(`e2ee_public_key_${userId}`),
        privatePem: localStorage.getItem(`e2ee_private_key_${userId}`),
    }),
    clearStoredKeys: (userId) => {
        localStorage.removeItem(`e2ee_public_key_${userId}`);
        localStorage.removeItem(`e2ee_private_key_${userId}`);
    },
    generateKeys: () => {
        const crypt = new window.JSEncrypt({ default_key_size: 2048 });
        crypt.getKey();
        return { publicKeyPem: crypt.getPublicKey(), privateKeyPem: crypt.getPrivateKey() };
    },
    ensureKeyPair: async (userId) => {
        const stored = e2e.loadStoredKeys(userId);
        if (stored.publicPem && stored.privatePem) {
            // Verify if keys are valid and matching
            const testPlain = 'verify-key-' + Date.now();
            const enc = e2e.encryptMessage(testPlain, stored.publicPem);
            if (enc) {
                const dec = e2e.decryptMessage(enc, stored.privatePem);
                if (dec === testPlain) {
                    console.log("[E2EE] Valid keys found in localStorage for user:", userId);
                    return { publicKeyPem: stored.publicPem, privateKeyPem: stored.privatePem };
                }
            }
            console.warn("[E2EE] Stored keys corrupt or invalid. Regenerating...");
            e2e.clearStoredKeys(userId);
        }

        console.log("[E2EE] Generating new RSA-2048 keys for user:", userId);
        const keys = e2e.generateKeys();
        e2e.storeKeys(userId, keys.publicKeyPem, keys.privateKeyPem);

        try {
            await api.users.updatePublicKey(keys.publicKeyPem);
            console.log("[E2EE] New public key synced to server.");
        } catch (err) {
            console.error("[E2EE] Failed to sync new public key to server:", err);
        }

        return keys;
    },
    getPublicKey: async (userId) => {
        if (publicKeyCache[userId]) return publicKeyCache[userId];
        try {
            const resp = await api.users.getPublicKey(userId);
            if (resp.data?.public_key) {
                publicKeyCache[userId] = resp.data.public_key;
                return resp.data.public_key;
            }
            throw new Error("No public key found in response");
        } catch (err) {
            console.error(`Failed to fetch public key for ${userId}`, err);
            return null;
        }
    },
    encryptMessage: (plaintext, publicKeyPem) => {
        if (!publicKeyPem) return false;
        const encryptor = new window.JSEncrypt();
        encryptor.setPublicKey(publicKeyPem);
        return encryptor.encrypt(plaintext);
    },
    decryptMessage: (ciphertext, privateKeyPem) => {
        if (!privateKeyPem) return false;
        const decryptor = new window.JSEncrypt();
        decryptor.setPrivateKey(privateKeyPem);
        return decryptor.decrypt(ciphertext);
    },
    /** @returns {{ kind:'dual', r:string, s:string }|{ kind:'legacy', raw:string }|{ kind:'empty' }} */
    parseContentEnvelope(raw) {
        if (typeof raw !== 'string' || raw.length === 0) return { kind: 'empty' };
        if (raw[0] !== '{') return { kind: 'legacy', raw };
        try {
            const o = JSON.parse(raw);
            if (o && o.v === 1 && typeof o.r === 'string' && typeof o.s === 'string')
                return { kind: 'dual', r: o.r, s: o.s };
        } catch (_) { /* not JSON */ }
        return { kind: 'legacy', raw };
    },
    /**
     * @param {string} contentEncrypted
     * @param {boolean} isSentByMe
     * @param {string} privateKeyPem
     * @returns {string|false|null} plaintext, false if decrypt failed, null if legacy self-sent (undecryptable)
     */
    decryptChatContent(contentEncrypted, isSentByMe, privateKeyPem) {
        const env = e2e.parseContentEnvelope(contentEncrypted);
        if (env.kind === 'empty') return '';
        if (!privateKeyPem) return false;
        if (env.kind === 'dual') {
            const blob = isSentByMe ? env.s : env.r;
            return e2e.decryptMessage(blob, privateKeyPem);
        }
        if (isSentByMe) return null;
        return e2e.decryptMessage(env.raw, privateKeyPem);
    }
};

// ---- WEBSOCKET ----
const WS_BASE = 'ws://localhost:8000/connection/websocket';
let centrifuge = null;
let currentSubscriptions = {};
let wsCallbacks = { onMessage: null, onPresence: null, onConnect: null, onDisconnect: null };

// Wired from DOMContentLoaded so channel join/leave handlers (defined above setOnlineStatus) can update dots.
const presenceBridge = {
    applyPresence(_userId, _status, _specificConvId) { /* replaced on init */ }
};

const ws = {
    setCallbacks: (cb) => { wsCallbacks = { ...wsCallbacks, ...cb }; },
    connect: (token) => {
        if (centrifuge) ws.disconnect();
        console.log('Centrifugo initializing...');
        centrifuge = new window.Centrifuge(WS_BASE, { token });
        centrifuge.on('connecting', () => console.log('Centrifugo connecting...'))
            .on('connected', (ctx) => { console.log(`Centrifugo connected via ${ctx.transport}`); if (wsCallbacks.onConnect) wsCallbacks.onConnect(); })
            .on('disconnected', (ctx) => { console.log(`Centrifugo disconnected: ${ctx.reason}`); if (wsCallbacks.onDisconnect) wsCallbacks.onDisconnect(); })
            .connect();
    },
    subscribe: (conversationId) => {
        const channel = `chat:${conversationId}`;
        if (currentSubscriptions[channel]) return currentSubscriptions[channel];
        const sub = centrifuge.newSubscription(channel);

        sub.on('publication', function (ctx) {
            const data = ctx.data;
            const msgType = data.type ?? data.Type;
            if (msgType === 'presence_update') {
                if (wsCallbacks.onPresence) wsCallbacks.onPresence(data);
            } else {
                if (wsCallbacks.onMessage) wsCallbacks.onMessage(data);
            }
        });

        // Optimization: Sync initial presence list when subscribed
        sub.on('subscribed', function (ctx) {
            console.log(`[Presence] Subscribed to ${channel}, syncing initial presence...`);
            sub.presence().then(function (result) {
                // Mapping: Result.clients contains connection infos keyed by client ID
                for (let clientID in result.clients) {
                    const info = result.clients[clientID];
                    // Map Centrifugo user ID to UI status
                    presenceBridge.applyPresence(info.user, 'ONLINE');
                }
                console.log("Current presence list updated");
            }).catch(err => console.error("Presence fetch error:", err));
        });

        // Handle real-time join/leave events
        sub.on('join', function (ctx) {
            console.log(`[Presence] User joined ${channel}:`, ctx.info.user);
            presenceBridge.applyPresence(ctx.info.user, 'ONLINE');
        });
        sub.on('leave', function (ctx) {
            console.log(`[Presence] User left ${channel}:`, ctx.info.user);
            presenceBridge.applyPresence(ctx.info.user, 'OFFLINE');
        });

        sub.subscribe();
        currentSubscriptions[channel] = sub;
        return sub;
    },
    getPresence: async (channel) => {
        if (!centrifuge) return {};
        try {
            const resp = await centrifuge.presence(channel);
            return resp.clients || {};
        } catch (err) {
            console.error("Failed to fetch presence for", channel, err);
            return {};
        }
    },
    subscribeUserChannel: (userId) => {
        const channel = `user:#${userId}`;
        if (currentSubscriptions[channel]) return currentSubscriptions[channel];
        console.log('Subscribing to personal channel', channel);
        const sub = centrifuge.newSubscription(channel);
        sub.on('publication', function (ctx) {
            if (wsCallbacks.onSystemEvent) wsCallbacks.onSystemEvent(ctx.data);
        });
        sub.subscribe();
        currentSubscriptions[channel] = sub;
        return sub;
    },
    disconnect: () => {
        // Explicitly unsubscribe and remove listeners from each subscription
        for (let channel in currentSubscriptions) {
            const sub = currentSubscriptions[channel];
            if (sub) {
                sub.unsubscribe();
                sub.removeAllListeners();
            }
        }
        currentSubscriptions = {};

        if (centrifuge) {
            centrifuge.disconnect();
            centrifuge.removeAllListeners();
            centrifuge = null;
        }
        console.log("Disconnected from Centrifugo");
    }
};

// ---- APP STATE & LOGIC ----
let currentUser = { id: null, username: null, keys: null };
let currentConversationId = null;
let conversationsObj = {};
let highestMessageTimestamp = null;
let oldestMessageTimestamp = null;
let isLoadingMore = false;

window.addEventListener('DOMContentLoaded', () => {
    /** Compare user ids from JWT vs API (UUID casing / string coercion). */
    function sameUserId(a, b) {
        if (a == null || b == null) return false;
        return String(a).trim().toLowerCase() === String(b).trim().toLowerCase();
    }

    // --- DOM REFERENCES ---
    const els = {
        overlayAuth: document.getElementById('auth-overlay'),
        usernameInp: document.getElementById('auth-username'),
        passwordInp: document.getElementById('auth-password'),
        btnLogin: document.getElementById('btn-login'),
        btnRegister: document.getElementById('btn-register'),
        authMsg: document.getElementById('auth-msg'),

        appContainer: document.getElementById('app-container'),
        myAvatar: document.getElementById('my-avatar'),
        myName: document.getElementById('my-name'),

        convList: document.getElementById('conv-list'),

        chatEmpty: document.getElementById('chat-empty'),
        chatView: document.getElementById('chat-view'),
        chatName: document.getElementById('current-chat-name'),
        chatStatus: document.getElementById('current-chat-status'),
        chatAvatar: document.getElementById('current-chat-avatar'),

        messagesContainer: document.getElementById('messages-container'),
        chatMessagesScroll: document.getElementById('chat-messages'),

        msgInput: document.getElementById('msg-input'),
        btnSend: document.getElementById('btn-send'),

        btnNewChat: document.getElementById('btn-new-chat'),
        dialogNewChat: document.getElementById('dialog-new-chat'),
        newGroupName: document.getElementById('new-group-name'),
        newGroupMembers: document.getElementById('new-group-members'),
        btnCancelGroup: document.getElementById('btn-cancel-group'),
        btnCreateGroup: document.getElementById('btn-create-group'),

        // Search & Friends
        searchInput: document.getElementById('search-input'),
        searchResults: document.getElementById('search-results'),
        btnFriendsPending: document.getElementById('btn-friends-pending'),
        dialogPending: document.getElementById('dialog-pending'),
        pendingList: document.getElementById('pending-list'),
        btnClosePending: document.getElementById('btn-close-pending'),

        loadingHistory: document.getElementById('loading-history'),
        // Friend Requests Section
        friendReqSection: document.getElementById('friend-requests-section'),
        friendReqList: document.getElementById('friend-requests-list'),

        btnLogout: document.getElementById('btn-logout'),
    };

    // --- EVENT BINDINGS ---
    els.btnRegister.addEventListener('click', handleRegister);
    els.btnLogin.addEventListener('click', handleLogin);
    els.btnLogout.addEventListener('click', handleLogout);
    els.btnSend.addEventListener('click', handleSend);
    els.msgInput.addEventListener('keypress', (e) => { if (e.key === 'Enter') handleSend(); });
    els.btnNewChat.addEventListener('click', () => els.dialogNewChat.classList.add('active'));
    els.btnCancelGroup.addEventListener('click', () => els.dialogNewChat.classList.remove('active'));
    els.btnCreateGroup.addEventListener('click', handleCreateGroup);
    els.chatMessagesScroll.addEventListener('scroll', handleScrollHistory);

    // Search input with 300ms debounce
    let searchDebounce = null;
    els.searchInput.addEventListener('input', () => {
        clearTimeout(searchDebounce);
        const q = els.searchInput.value.trim();
        if (!q) { els.searchResults.style.display = 'none'; return; }
        searchDebounce = setTimeout(() => handleSearch(q), 300);
    });
    // Close search results when clicking outside
    document.addEventListener('click', (e) => {
        if (!els.searchInput.contains(e.target) && !els.searchResults.contains(e.target)) {
            els.searchResults.style.display = 'none';
        }
    });

    // Pending requests dialog
    els.btnFriendsPending.addEventListener('click', handleOpenPending);
    els.btnClosePending.addEventListener('click', () => { els.dialogPending.classList.remove('active'); });

    // Close dialogs on overlay click
    els.dialogNewChat.addEventListener('click', (e) => { if (e.target === els.dialogNewChat) els.dialogNewChat.classList.remove('active'); });
    els.dialogPending.addEventListener('click', (e) => { if (e.target === els.dialogPending) els.dialogPending.classList.remove('active'); });

    let pendingBadgeCount = 0;
    let friendRequests = [];

    ws.setCallbacks({
        onConnect: () => {
            console.log('WS Connected');
            // Subscribe to personal user channel for system events
            if (currentUser.id) ws.subscribeUserChannel(currentUser.id);
        },
        onDisconnect: () => console.log('WS Disconnected'),
        onMessage: handleIncomingMessage,
        onPresence: handleIncomingPresence,
        onSystemEvent: handleSystemEvent
    });

    function showAuthError(msg) { els.authMsg.textContent = msg; }

    async function handleRegister() {
        const user = els.usernameInp.value.trim();
        const pass = els.passwordInp.value.trim();
        if (!user || !pass) return showAuthError("Username & Password required");

        try {
            els.btnRegister.disabled = true;
            els.btnRegister.textContent = "Generating keys...";

            let keys = e2e.generateKeys();
            await api.auth.register(user, pass, keys.publicKeyPem);

            // Note: We don't store keys during registration to ensure the login flow 
            // is the source of truth for current logged-in user context.

            showAuthError("Registered successfully! Please login.");
            els.btnRegister.textContent = "Create new account";
            els.btnRegister.disabled = false;
        } catch (err) {
            showAuthError(err.message);
            els.btnRegister.textContent = "Create new account";
            els.btnRegister.disabled = false;
        }
    }

    async function handleLogin() {
        const user = els.usernameInp.value.trim();
        const pass = els.passwordInp.value.trim();
        if (!user || !pass) return showAuthError("Username & Password required");

        try {
            els.btnLogin.disabled = true;
            els.btnLogin.textContent = "Logging in...";

            const loginResp = await api.auth.login(user, pass);
            const token = loginResp.data;
            setToken(token);

            const payload = JSON.parse(atob(token.split('.')[1]));
            const userId = String(payload.user_id || payload.sub || '').trim();
            if (!userId) throw new Error('Invalid token: missing user id');

            // Strictly check/gen key pair on login
            const keys = await e2e.ensureKeyPair(userId);
            currentUser = { id: userId, username: user, keys };

            els.myAvatar.textContent = user.charAt(0).toUpperCase();
            els.myName.textContent = user;
            els.overlayAuth.classList.add('hidden');
            els.appContainer.classList.add('active');

            ws.connect(token);
            await loadDashboard();

        } catch (err) {
            showAuthError(err.message);
            els.btnLogin.disabled = false;
            els.btnLogin.textContent = "Login";
        }
    }

    async function handleLogout() {
        if (!confirm("Are you sure you want to logout?")) return;

        // Reset UI Presence items
        clearAllPresenceDots();

        ws.disconnect();
        setToken('');
        currentUser = { id: null, username: null, keys: null };
        conversationsObj = {};
        currentConversationId = null;
        for (const k of Object.keys(publicKeyCache)) delete publicKeyCache[k];

        els.appContainer.classList.remove('active');
        els.overlayAuth.classList.remove('hidden');
        els.btnLogin.disabled = false;
        els.btnLogin.textContent = "Login";

        console.log("[Auth] Logged out. State cleared. E2EE keys preserved in localStorage.");
    }

    async function loadDashboard() {
        try {
            const resp = await api.conversations.list();
            const convs = resp.data || [];
            let participantIds = new Set();
            els.convList.innerHTML = '';
            conversationsObj = {};

            // Use Promise.all to ensure all conversations are initialized before proceeding
            await Promise.all(convs.map(async (conv) => {
                conversationsObj[conv.id] = conv;
                ws.subscribe(conv.id);

                if (conv.participants) {
                    conv.participants.forEach(p => { 
                        if (!sameUserId(p.user_id, currentUser.id)) participantIds.add(p.user_id); 
                    });
                }
                renderSidebarItem(conv);

                // Fetch immediate presence for this channel from the server
                try {
                    const presence = await ws.getPresence(`chat:${conv.id}`);
                    for (let clientID in presence) {
                        const info = presence[clientID];
                        updatePresenceUI(info.user, 'ONLINE', conv.id);
                    }
                } catch (e) {
                    console.warn(`Could not fetch presence for conv ${conv.id}`, e);
                }
            }));

            // Note: We skip the fallback Redis API presence fetch here because JWT client proxy
            // webhooks in Centrifugo are not invoked by default, making Redis presence stale.
            // Centrifugo channel presence is our single source of truth for real-time status.
            await fetchFriendRequests();

        } catch (err) { console.error("Failed to load dashboard", err); }
    }

    function renderSidebarItem(conv) {
        const elId = `conv-${conv.id}`;
        let el = document.getElementById(elId);
        if (!el) {
            el = document.createElement('div');
            el.className = 'conv-item';
            el.id = elId;
            el.onclick = () => openConversation(conv.id);
            let avatarLetter = (conv.name || "?").charAt(0).toUpperCase();
            el.innerHTML = `
                <div class="conv-item-avatar">
                    <div class="avatar-placeholder" style="width: 44px; height: 44px;">${avatarLetter}</div>
                    <div class="presence-dot" id="presence-${conv.id}"></div>
                </div>
                <div class="conv-item-details">
                    <div class="conv-item-top">
                        <span class="conv-item-name">${conv.name || "Conversation"}</span>
                        <span class="conv-item-time" id="time-${conv.id}"></span>
                    </div>
                    <div class="conv-item-last-msg" id="last-msg-${conv.id}">No messages yet</div>
                </div>
            `;
            els.convList.appendChild(el);
        }
    }

    /**
     * Centralized function to update user online status across all UI components.
     * Maps user_id from Centrifugo/Backend to the corresponding elements in the Sidebar.
     */
    function setOnlineStatus(userId, status, specificConvId = null) {
        const isOnline = String(status || '').toLowerCase() === 'online';
        
        // Helper to update a single dot
        const updateDot = (cid) => {
            let dot = document.getElementById(`presence-${cid}`);
            if (!dot) return;
            
            if (isOnline) {
                dot.classList.remove('bg-gray-400');
                dot.classList.add('bg-green-500', 'online');
            } else {
                dot.classList.remove('bg-green-500', 'online');
                dot.classList.add('bg-gray-400');
            }

            // Also update the active chat header if this is the current conversation
            if (currentConversationId === cid) {
                els.chatStatus.innerHTML = isOnline ? '🟢 Online' : '⚪ Offline';
            }
        };

        if (specificConvId) {
            updateDot(specificConvId);
            return;
        }

        // --- IMPORTANT LOGIC CHANGE ---
        // If the user who joined/left is ME, we don't need to update any coworker dots in the sidebar.
        // The dots represent the status of the peer. My own status is shown in the sidebar header.
        if (currentUser.id && sameUserId(userId, currentUser.id)) {
             console.log("[Presence] Ignoring self-presence update for sidebar dots.");
             return;
        }

        // Iterate through all conversations to find where this user belongs
        for (let cid in conversationsObj) {
            let conv = conversationsObj[cid];
            let hasUser = conv.participants && conv.participants.some(p => sameUserId(p.user_id, userId));
            if (hasUser) updateDot(cid);
        }
    }

    // Keep legacy wrapper for compatibility
    function updatePresenceUI(userId, status, specificConvId = null) { setOnlineStatus(userId, status, specificConvId); }

    presenceBridge.applyPresence = setOnlineStatus;

    function clearAllPresenceDots() {
        const dots = document.querySelectorAll('.presence-dot');
        dots.forEach(dot => {
            dot.classList.remove('bg-green-500', 'online');
            dot.classList.add('bg-gray-400');
        });
        if (els.chatStatus) els.chatStatus.innerHTML = '⚪ Offline';
        console.log("[Presence] All UI presence status cleared.");
    }

    async function handleCreateGroup() {
        const name = els.newGroupName.value.trim();
        const membersRaw = els.newGroupMembers.value.trim();
        if (!name || !membersRaw) return;
        let members = membersRaw.split(',').map(m => m.trim()).filter(x => x);
        try {
            els.btnCreateGroup.disabled = true;
            await api.conversations.createGroup(name, members);
            els.dialogNewChat.classList.remove('active');
            els.newGroupName.value = '';
            els.newGroupMembers.value = '';
            await loadDashboard();
        } catch (err) { alert(err.message); }
        finally { els.btnCreateGroup.disabled = false; }
    }

    async function openConversation(cid) {
        if (currentConversationId) {
            const p = document.getElementById(`conv-${currentConversationId}`);
            if (p) p.classList.remove('active');
        }
        currentConversationId = cid;
        const item = document.getElementById(`conv-${currentConversationId}`);
        if (item) item.classList.add('active');

        const conv = conversationsObj[cid];
        els.chatAvatar.textContent = (conv.name || "?").charAt(0).toUpperCase();
        els.chatName.textContent = conv.name || "Conversation";
        let dot = document.getElementById(`presence-${cid}`);
        let isOnline = dot && dot.classList.contains('online');
        els.chatStatus.innerHTML = isOnline ? '🟢 Online' : '⚪ Offline';

        els.chatEmpty.style.display = 'none';
        els.chatView.style.display = 'flex';
        els.btnSend.disabled = false;
        els.messagesContainer.innerHTML = '';
        oldestMessageTimestamp = null;
        highestMessageTimestamp = null;

        await loadMessages(cid, null);
        scrollToBottom();
    }

    async function loadMessages(cid, beforeTs) {
        isLoadingMore = true;
        if (beforeTs) els.loadingHistory.classList.remove('hidden');
        try {
            const resp = await api.conversations.getMessages(cid, 100, beforeTs);
            const msgs = resp.data || [];
            if (msgs.length > 0) {
                // API returns newest-first (DESC). Prepend in that order so DOM ends up oldest→newest (top→bottom).
                oldestMessageTimestamp = new Date(msgs[msgs.length - 1].created_at).getTime();
                msgs.forEach(m => prependMessageToUI(m));
            }
        } catch (err) { console.error("Failed to fetch messages", err); }
        finally { els.loadingHistory.classList.add('hidden'); isLoadingMore = false; }
    }

    function prependMessageToUI(m) {
        const isSent = sameUserId(m.sender_id, currentUser.id);
        let text = "";
        if (!currentUser.keys || !currentUser.keys.privateKeyPem) {
            text = "⚠️ Vui lòng thiết lập khóa bảo mật để đọc tin nhắn này";
            console.warn("[E2EE] Decryption skipped: Private key missing.");
        } else {
            const dec = e2e.decryptChatContent(m.content_encrypted, isSent, currentUser.keys.privateKeyPem);
            if (dec !== false && dec !== null) {
                text = dec;
            } else if (dec === null) {
                text = "🔒 Tin bạn đã gửi (định dạng cũ — chỉ người nhận đọc được)";
            } else {
                text = "⚠️ [Conversation corrupted - RSA decryption failed]";
                console.error("[E2EE] RSA decryption failed for message:", m.message_id);
            }
        }
        const timeStr = new Date(m.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const div = document.createElement('div');
        div.className = `msg-row ${isSent ? 'sent' : 'received'}`;
        div.innerHTML = `
            <div class="msg-bubble">${text}</div>
            <div class="msg-meta">
                ${isSent ? '<span class="e2e-icon">🔒</span>' : ''}
                <span>${timeStr}</span>
                ${!isSent ? '<span class="e2e-icon">🔓</span>' : ''}
            </div>
        `;
        els.messagesContainer.insertBefore(div, els.messagesContainer.firstChild);
    }

    function appendMessageToUI(senderId, content, dateObj, isSent) {
        const timeStr = dateObj.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        const div = document.createElement('div');
        div.className = `msg-row ${isSent ? 'sent' : 'received'}`;
        div.innerHTML = `
            <div class="msg-bubble">${content}</div>
            <div class="msg-meta">
                ${isSent ? '<span class="e2e-icon">🔒</span>' : ''}
                <span>${timeStr}</span>
                ${!isSent ? '<span class="e2e-icon">🔓</span>' : ''}
            </div>
        `;
        els.messagesContainer.appendChild(div);
        scrollToBottom();
    }

    async function handleScrollHistory() {
        if (isLoadingMore || !currentConversationId || !oldestMessageTimestamp) return;
        if (els.chatMessagesScroll.scrollTop === 0) {
            const oldHeight = els.chatMessagesScroll.scrollHeight;
            await loadMessages(currentConversationId, oldestMessageTimestamp);
            const newHeight = els.chatMessagesScroll.scrollHeight;
            els.chatMessagesScroll.scrollTop = newHeight - oldHeight;
        }
    }

    function scrollToBottom() { els.chatMessagesScroll.scrollTop = els.chatMessagesScroll.scrollHeight; }

    async function handleSend() {
        const text = els.msgInput.value.trim();
        if (!text || !currentConversationId) return;
        const conv = conversationsObj[currentConversationId];
        if (!conv || !conv.participants) return alert("Conversation corrupted");
        const receiver = conv.participants.find(p => !sameUserId(p.user_id, currentUser.id));
        if (!receiver) return alert("No receiver found in this conversation");
        try {
            els.btnSend.disabled = true;
            console.log("Encrypting with Public Key of user:", receiver.user_id);
            const pubKey = await e2e.getPublicKey(receiver.user_id);
            if (!pubKey) throw new Error("Could not fetch receiver's public key (they might need to set one up)");
            const ownPub = currentUser.keys.publicKeyPem;
            if (!ownPub) throw new Error("Missing your public key — try logging in again");
            const encR = e2e.encryptMessage(text, pubKey);
            const encS = e2e.encryptMessage(text, ownPub);
            if (!encR || !encS) throw new Error("RSA Encryption failed (message too long or bad key format)");
            const encryptedText = JSON.stringify({ v: 1, r: encR, s: encS });
            await api.chat.sendMessage(currentConversationId, encryptedText);
            appendMessageToUI(currentUser.id, text, new Date(), true);
            updateSidebarLastMsg(currentConversationId, "You: " + text, new Date());
            els.msgInput.value = '';
        } catch (err) { alert("Failed to send: " + err.message); }
        finally { els.btnSend.disabled = false; els.msgInput.focus(); }
    }

    function handleIncomingMessage(data) {
        const cid = data.conversation_id;
        if (sameUserId(data.sender_id, currentUser.id)) return;

        let text = "";
        if (!currentUser.keys || !currentUser.keys.privateKeyPem) {
            text = "⚠️ Vui lòng thiết lập khóa bảo mật để đọc tin nhắn này";
            console.warn("[E2EE] Incoming message decryption skipped: Private key missing.");
        } else {
            const decText = e2e.decryptChatContent(data.content_encrypted, false, currentUser.keys.privateKeyPem);
            if (decText !== false && decText !== null) {
                text = decText;
            } else {
                text = "⚠️ [Conversation corrupted - RSA decryption failed]";
                console.error("[E2EE] RSA decryption failed for incoming message:", data.message_id);
            }
        }

        if (cid === currentConversationId) appendMessageToUI(data.sender_id, text, new Date(data.created_at), false);
        updateSidebarLastMsg(cid, text, new Date(data.created_at));
    }

    function handleIncomingPresence(data) {
        const uid = data.user_id ?? data.UserID;
        const st = data.status ?? data.Status;
        updatePresenceUI(uid, st);
    }

    // --- SEARCH HANDLERS ---

    async function handleSearch(query) {
        try {
            const resp = await api.users.search(query);
            const users = resp.data || [];
            els.searchResults.innerHTML = '';
            if (users.length === 0) {
                els.searchResults.innerHTML = '<div style="padding: 10px; color: var(--text-muted); font-size: 0.85rem;">No users found</div>';
            } else {
                users.forEach(u => {
                    const item = document.createElement('div');
                    item.style.cssText = 'padding: 8px 12px; display: flex; justify-content: space-between; align-items: center; cursor: pointer; border-bottom: 1px solid var(--border);';
                    item.innerHTML = `
                        <div>
                            <div style="font-weight: 600; font-size: 0.88rem;">${escapeHtml(u.username)}</div>
                            <div style="font-size: 0.75rem; color: var(--text-muted);">${u.public_key ? '🔑 Has public key' : '⚠️ No E2EE key'}</div>
                        </div>
                        <button onclick="handleAddFriend('${u.id}')"
                            style="background: var(--primary); border: none; color: white; padding: 4px 10px; border-radius: 4px; font-size: 0.78rem; cursor: pointer;">
                            + Add Friend
                        </button>
                    `;
                    // Pre-cache public key for E2EE
                    if (u.public_key) publicKeyCache[u.id] = u.public_key;
                    els.searchResults.appendChild(item);
                });
            }
            els.searchResults.style.display = 'block';
        } catch (err) {
            console.error('Search failed:', err);
        }
    }

    // Exposed globally so inline onclick can call it
    window.handleAddFriend = async function (targetUserId) {
        try {
            await api.friendships.request(targetUserId);
            els.searchResults.style.display = 'none';
            els.searchInput.value = '';
            showToast('Friend request sent! ✅');
        } catch (err) {
            showToast('Error: ' + err.message, true);
        }
    };

    // --- FRIEND REQUESTS SIDEBAR LOGIC ---

    async function fetchFriendRequests() {
        try {
            const resp = await api.friendships.pending();
            friendRequests = resp.data || [];
            updateFriendRequestsUI();
        } catch (err) {
            console.error('Failed to fetch friend requests:', err);
        }
    }

    function updateFriendRequestsUI() {
        els.friendReqList.innerHTML = '';
        if (friendRequests.length === 0) {
            els.friendReqSection.style.display = 'none';
        } else {
            els.friendReqSection.style.display = 'block';
            friendRequests.forEach(req => {
                const card = renderFriendRequestCard(req);
                els.friendReqList.appendChild(card);
            });
        }
    }

    function renderFriendRequestCard(req) {
        const div = document.createElement('div');
        div.className = 'friend-req-card';
        div.id = `req-card-${req.id}`;

        const username = req.requester?.username || 'Unknown';
        const initial = username.charAt(0).toUpperCase();

        div.innerHTML = `
            <div class="friend-req-info">
                <div class="friend-req-avatar">${initial}</div>
                <div class="friend-req-user">
                    <div class="friend-req-username">${escapeHtml(username)}</div>
                </div>
            </div>
            <div class="friend-req-actions">
                <button class="friend-req-btn friend-req-btn-accept" id="accept-${req.id}">Accept</button>
                <button class="friend-req-btn friend-req-btn-reject" id="reject-${req.id}">Ignore</button>
            </div>
        `;

        const acceptBtn = div.querySelector(`#accept-${req.id}`);
        const rejectBtn = div.querySelector(`#reject-${req.id}`);

        acceptBtn.onclick = () => handleFriendRequestAction(req.id, 'accept');
        rejectBtn.onclick = () => handleFriendRequestAction(req.id, 'reject');

        return div;
    }

    async function handleFriendRequestAction(requestId, action) {
        const acceptBtn = document.getElementById(`accept-${requestId}`);
        const rejectBtn = document.getElementById(`reject-${requestId}`);

        if (!acceptBtn || !rejectBtn) return;

        // Loading state
        acceptBtn.disabled = true;
        rejectBtn.disabled = true;
        const originalText = action === 'accept' ? 'Accept' : 'Ignore';
        const targetBtn = action === 'accept' ? acceptBtn : rejectBtn;
        targetBtn.innerHTML = '<span class="friend-req-loading"></span>';

        try {
            if (action === 'accept') {
                await api.friendships.accept(requestId);
                showToast('Friend request accepted! 🎉');
                // Refresh dashboard to show new conversation
                await loadDashboard();
            } else {
                await api.friendships.reject(requestId);
                showToast('Friend request ignored.');
            }

            // Success: Remove request from state and update UI
            friendRequests = friendRequests.filter(r => r.id !== requestId);
            updateFriendRequestsUI();

        } catch (err) {
            showToast('Error: ' + err.message, true);
            // Re-enable on error
            acceptBtn.disabled = false;
            rejectBtn.disabled = false;
            targetBtn.textContent = originalText;
        }
    }

    // --- PENDING REQUESTS HANDLERS ---

    async function handleOpenPending() {
        try {
            const resp = await api.friendships.pending();
            const requests = resp.data || [];
            els.pendingList.innerHTML = '';
            pendingBadgeCount = 0;
            updatePendingBadge();

            if (requests.length === 0) {
                els.pendingList.innerHTML = '<div style="color: var(--text-muted); font-size: 0.88rem; text-align: center; padding: 20px;">No pending requests 🎉</div>';
            } else {
                requests.forEach(req => {
                    const item = document.createElement('div');
                    item.style.cssText = 'padding: 10px; background: var(--bg-base); border-radius: 6px; display: flex; justify-content: space-between; align-items: center;';
                    const requesterName = req.requester?.username || req.requester_id;
                    item.innerHTML = `
                        <div>
                            <div style="font-weight: 600;">${escapeHtml(requesterName)}</div>
                            <div style="font-size: 0.75rem; color: var(--text-muted);">Wants to be your friend</div>
                        </div>
                        <div style="display: flex; gap: 6px;">
                            <button onclick="handleAcceptRequest('${req.id}')"
                                style="background: var(--success, #22c55e); border: none; color: white; padding: 4px 10px; border-radius: 4px; cursor: pointer; font-size: 0.8rem;">
                                ✓ Accept
                            </button>
                            <button onclick="handleRejectRequest('${req.id}')"
                                style="background: var(--bg-card); border: 1px solid var(--border); color: var(--text-muted); padding: 4px 10px; border-radius: 4px; cursor: pointer; font-size: 0.8rem;">
                                ✗ Reject
                            </button>
                        </div>
                    `;
                    els.pendingList.appendChild(item);
                });
            }
            els.dialogPending.classList.add('active');
        } catch (err) {
            console.error('Failed to load pending requests:', err);
        }
    }

    window.handleAcceptRequest = async function (requestId) {
        try {
            await api.friendships.accept(requestId);
            showToast('Friend request accepted! A new chat has been created 🎉');
            els.dialogPending.classList.remove('active');
            // Reload (the real-time event will also fire, but this is a good fallback)
            await loadDashboard();
        } catch (err) {
            showToast('Error: ' + err.message, true);
        }
    };

    window.handleRejectRequest = async function (requestId) {
        try {
            await api.friendships.reject(requestId);
            // Re-open dialog to refresh list
            await handleOpenPending();
        } catch (err) {
            showToast('Error: ' + err.message, true);
        }
    };

    // --- SYSTEM EVENT HANDLER (Centrifugo user:#id channel) ---

    function handleSystemEvent(data) {
        console.log('[SystemEvent]', data);
        switch (data.type) {
            case 'friend_request_received':
                pendingBadgeCount++;
                updatePendingBadge();
                showToast('You have a new friend request! 👋');
                fetchFriendRequests(); // Sync sidebar section
                break;

            case 'conversation_created':
                handleNewConversationCreated(data);
                break;

            default:
                console.log('[SystemEvent] Unknown type:', data.type);
        }
    }

    async function handleNewConversationCreated(data) {
        const { conversation_id, peer_id, peer_name, peer_public_key, c_type } = data;

        // Cache the peer's public key immediately — E2EE ready from message 1!
        if (peer_id && peer_public_key) {
            publicKeyCache[peer_id] = peer_public_key;
            console.log(`[E2EE] Cached public key for peer ${peer_name} (${peer_id})`);
        }

        // Avoid duplicates
        if (conversationsObj[conversation_id]) return;

        // Build a minimal conversation object with enough info to render
        const conv = {
            id: conversation_id,
            name: peer_name || 'Direct Chat',
            type: c_type || 'DIRECT',
            participants: [
                { user_id: currentUser.id },
                { user_id: peer_id }
            ]
        };

        conversationsObj[conversation_id] = conv;
        ws.subscribe(conversation_id);

        // Fetch presence immediately for the new chat
        const presence = await ws.getPresence(`chat:${conversation_id}`);
        for (let clientID in presence) {
            const info = presence[clientID];
            if (info && info.user) updatePresenceUI(info.user, 'online', conversation_id);
        }

        renderSidebarItem(conv);

        showToast(`New chat with ${peer_name || 'someone'} started! 💬`);
    }

    // --- UTILITY HELPERS ---

    function updatePendingBadge() {
        const badge = pendingBadgeCount > 0 ? ` (${pendingBadgeCount})` : '';
        els.btnFriendsPending.textContent = '🔔' + badge;
    }

    function showToast(msg, isError = false) {
        let toast = document.getElementById('app-toast');
        if (!toast) {
            toast = document.createElement('div');
            toast.id = 'app-toast';
            toast.style.cssText = `
                position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
                background: var(--bg-card, #1e1e2e); border: 1px solid var(--border, #333);
                color: var(--text, #fff); padding: 10px 20px; border-radius: 8px;
                font-size: 0.88rem; z-index: 9999; box-shadow: 0 4px 16px rgba(0,0,0,0.4);
                transition: opacity 0.3s ease;
            `;
            document.body.appendChild(toast);
        }
        toast.textContent = msg;
        toast.style.opacity = '1';
        toast.style.background = isError ? '#7f1d1d' : 'var(--bg-card, #1e1e2e)';
        clearTimeout(toast._timeout);
        toast._timeout = setTimeout(() => { toast.style.opacity = '0'; }, 3500);
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.appendChild(document.createTextNode(str || ''));
        return div.innerHTML;
    }

    function updateSidebarLastMsg(cid, text, dateObj) {
        const msgEl = document.getElementById(`last-msg-${cid}`);
        const timeEl = document.getElementById(`time-${cid}`);
        if (msgEl) msgEl.textContent = text;
        if (timeEl) {
            const now = new Date();
            const isToday = now.toDateString() === dateObj.toDateString();
            timeEl.textContent = isToday ? dateObj.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : dateObj.toLocaleDateString();
        }
        const convEl = document.getElementById(`conv-${cid}`);
        if (convEl && els.convList.firstChild !== convEl) els.convList.prepend(convEl);
    }
});
