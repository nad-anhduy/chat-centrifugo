import { api } from './api.js';

const publicKeyCache = {};

function pemToDer(pem) {
    const b64 = pem.replace(/-----BEGIN [^-]+-----|-----END [^-]+-----|\s/g, '');
    const raw = atob(b64);
    const buf = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) buf[i] = raw.charCodeAt(i);
    return buf.buffer;
}

function b64encode(buf) {
    const bytes = new Uint8Array(buf);
    let bin = '';
    const chunk = 8192;
    for (let i = 0; i < bytes.length; i += chunk) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
    }
    return btoa(bin);
}

function b64decode(s) {
    const raw = atob(s);
    const u = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) u[i] = raw.charCodeAt(i);
    return u;
}

async function importSpkiPublicRSA(pem) {
    return crypto.subtle.importKey(
        'spki',
        pemToDer(pem),
        { name: 'RSA-OAEP', hash: 'SHA-256' },
        false,
        ['encrypt']
    );
}

async function importPkcs8PrivateRSA(pem) {
    return crypto.subtle.importKey(
        'pkcs8',
        pemToDer(pem),
        { name: 'RSA-OAEP', hash: 'SHA-256' },
        false,
        ['decrypt']
    );
}

/** AES-GCM + RSA-OAEP hybrid: ciphertext + wrapped AES key + IV (all base64). */
export async function encryptHybridForPeer(plaintext, receiverPublicKeyPem, senderPublicKeyPem) {
    const receiverPub = await importSpkiPublicRSA(receiverPublicKeyPem);
    const senderPub = senderPublicKeyPem ? await importSpkiPublicRSA(senderPublicKeyPem) : null;
    const aesRaw = crypto.getRandomValues(new Uint8Array(32));
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const aesKey = await crypto.subtle.importKey('raw', aesRaw, { name: 'AES-GCM', length: 256 }, false, ['encrypt']);
    const pt = new TextEncoder().encode(plaintext);
    const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, aesKey, pt);
    const wrappedForReceiver = await crypto.subtle.encrypt({ name: 'RSA-OAEP' }, receiverPub, aesRaw);
    const wrappedForSender = senderPub ? await crypto.subtle.encrypt({ name: 'RSA-OAEP' }, senderPub, aesRaw) : null;
    return {
        content_encrypted: b64encode(ct),
        key_for_receiver: b64encode(wrappedForReceiver),
        key_for_sender: wrappedForSender ? b64encode(wrappedForSender) : '',
        iv: b64encode(iv),
    };
}

export async function decryptHybridMessage(contentB64, wrapB64, ivB64, privateKeyPem) {
    const priv = await importPkcs8PrivateRSA(privateKeyPem);
    const wrap = b64decode(wrapB64);
    const iv = b64decode(ivB64);
    const ct = b64decode(contentB64);
    const aesRawBuf = await crypto.subtle.decrypt({ name: 'RSA-OAEP' }, priv, wrap);
    const aesKey = await crypto.subtle.importKey(
        'raw',
        new Uint8Array(aesRawBuf),
        { name: 'AES-GCM', length: 256 },
        false,
        ['decrypt']
    );
    const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, aesKey, ct);
    return new TextDecoder().decode(pt);
}

export const e2e = {
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

    /** After login, persist server-issued PEM keys locally. */
    applyLoginKeys: (userId, publicKeyPem, privateKeyPem) => {
        e2e.storeKeys(userId, publicKeyPem, privateKeyPem);
        return { publicKeyPem, privateKeyPem };
    },

    getPublicKey: async (userId) => {
        if (publicKeyCache[userId]) return publicKeyCache[userId];
        try {
            const resp = await api.users.getPublicKey(userId);
            if (resp.data && resp.data.public_key) {
                publicKeyCache[userId] = resp.data.public_key;
                return resp.data.public_key;
            }
            throw new Error('No public key found in response');
        } catch (err) {
            console.error(`Failed to fetch public key for ${userId}`, err);
            return null;
        }
    },

    encryptMessage: (plaintext, publicKeyPem) => {
        if (!publicKeyPem || !window.JSEncrypt) return false;
        const encryptor = new window.JSEncrypt();
        encryptor.setPublicKey(publicKeyPem);
        return encryptor.encrypt(plaintext);
    },

    decryptMessage: (ciphertext, privateKeyPem) => {
        if (!privateKeyPem || !window.JSEncrypt) return false;
        const decryptor = new window.JSEncrypt();
        decryptor.setPrivateKey(privateKeyPem);
        return decryptor.decrypt(ciphertext);
    },

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
    },
};
