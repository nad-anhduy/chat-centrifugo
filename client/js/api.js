export const API_BASE = 'http://localhost:8080/api/v1';

let authToken = '';

export function setToken(token) {
    authToken = token;
}

export function getToken() {
    return authToken;
}

async function request(path, options = {}) {
    const headers = { 'Content-Type': 'application/json', ...options.headers };
    if (authToken) {
        headers['Authorization'] = `Bearer ${authToken}`;
    }

    const response = await fetch(`${API_BASE}${path}`, { ...options, headers });
    const data = await response.json().catch(() => ({}));
    
    if (!response.ok) {
        throw new Error(data.error || `HTTP Error ${response.status}`);
    }
    return data;
}

export const api = {
    auth: {
        login: async (username, password) => {
            return request('/auth/login', {
                method: 'POST',
                body: JSON.stringify({ username, password })
            });
        },
        register: async (username, password) => {
            return request('/auth/register', {
                method: 'POST',
                body: JSON.stringify({ username, password })
            });
        }
    },
    users: {
        updatePublicKey: async (public_key) => {
            return request('/users/me/public-key', {
                method: 'PUT',
                body: JSON.stringify({ public_key })
            });
        },
        getPublicKey: async (userId) => {
            return request(`/users/${userId}/public-key`, { method: 'GET' });
        },
        getPresence: async (userIds) => {
            // GET /api/v1/users/presence?user_ids=id1,id2
            const query = userIds.join(',');
            return request(`/users/presence?user_ids=${query}`, { method: 'GET' });
        },
        search: async (query) => {
            return request(`/users/search?q=${encodeURIComponent(query)}`, { method: 'GET' });
        }
    },
    conversations: {
        list: async () => {
            return request('/conversations', { method: 'GET' });
        },
        createGroup: async (name, member_ids) => {
            return request('/conversations', {
                method: 'POST',
                body: JSON.stringify({ name, member_ids })
            });
        },
        getMessages: async (conversationId, limit = 100, before_ts = null) => {
            let url = `/conversations/${conversationId}/messages?limit=${limit}`;
            if (before_ts) {
                url += `&before_ts=${before_ts}`;
            }
            return request(url, { method: 'GET' });
        }
    },
    chat: {
        sendMessage: async (conversation_id, content_encrypted, key_for_sender = '', key_for_receiver = '', iv = '') => {
            return request('/chat/messages', {
                method: 'POST',
                body: JSON.stringify({ conversation_id, content_encrypted, key_for_sender, key_for_receiver, iv })
            });
        }
    },
    friendships: {
        request: async (target_user_id) => {
            return request('/friendships/request', {
                method: 'POST',
                body: JSON.stringify({ target_user_id })
            });
        },
        accept: async (request_id) => {
            return request('/friendships/accept', {
                method: 'POST',
                body: JSON.stringify({ request_id })
            });
        },
        reject: async (request_id) => {
            return request('/friendships/reject', {
                method: 'POST',
                body: JSON.stringify({ request_id })
            });
        },
        pending: async () => {
            return request('/friendships/pending', { method: 'GET' });
        }
    }
};
