import { api } from './api.js';

// Cache for public keys
const publicKeyCache = {};

export const e2e = {

    /**
     * Store keys in localStorage, scoped by user_id
     */
    storeKeys: (userId, publicPem, privatePem) => {
        localStorage.setItem(`e2ee_public_key_${userId}`, publicPem);
        localStorage.setItem(`e2ee_private_key_${userId}`, privatePem);
    },

    loadStoredKeys: (userId) => {
        return {
            publicPem: localStorage.getItem(`e2ee_public_key_${userId}`),
            privatePem: localStorage.getItem(`e2ee_private_key_${userId}`),
        };
    },

    clearStoredKeys: (userId) => {
        localStorage.removeItem(`e2ee_public_key_${userId}`);
        localStorage.removeItem(`e2ee_private_key_${userId}`);
    },

    /**
     * Generate new RSA 2048 keypair
     */
    generateKeys: () => {
        const crypt = new window.JSEncrypt({ default_key_size: 2048 });
        crypt.getKey();
        return {
            publicKeyPem: crypt.getPublicKey(),
            privateKeyPem: crypt.getPrivateKey(),
        };
    },

    /**
     * Ensure current user has a valid keypair. If not, generate and register.
     */
    ensureKeyPair: async (userId) => {
        const stored = e2e.loadStoredKeys(userId);
        if (stored.publicPem && stored.privatePem) {
            // Verify they work
            const testPlain = 'test';
            const enc = e2e.encryptMessage(testPlain, stored.publicPem);
            if (enc) {
                const dec = e2e.decryptMessage(enc, stored.privatePem);
                if (dec === testPlain) {
                    return { publicKeyPem: stored.publicPem, privateKeyPem: stored.privatePem };
                }
            }
            console.warn("Stored keys corrupt. Regenerating.");
            e2e.clearStoredKeys(userId);
        }

        console.log("Generating new RSA-2048 keys...");
        const keys = e2e.generateKeys();
        e2e.storeKeys(userId, keys.publicKeyPem, keys.privateKeyPem);

        // Sync to server
        await api.users.updatePublicKey(keys.publicKeyPem);
        return keys;
    },

    /**
     * Get public key for a user (from cache or API)
     */
    getPublicKey: async (userId) => {
        if (publicKeyCache[userId]) return publicKeyCache[userId];
        try {
            const resp = await api.users.getPublicKey(userId);
            if (resp.data && resp.data.public_key) {
                publicKeyCache[userId] = resp.data.public_key;
                return resp.data.public_key;
            }
            throw new Error("No public key found in response");
        } catch (err) {
            console.error(`Failed to fetch public key for ${userId}`, err);
            return null;
        }
    },

    /**
     * Encrypt message with a receiver's public key
     */
    encryptMessage: (plaintext, publicKeyPem) => {
        if (!publicKeyPem) return false;
        const encryptor = new window.JSEncrypt();
        encryptor.setPublicKey(publicKeyPem);
        return encryptor.encrypt(plaintext); 
    },

    /**
     * Decrypt message with my private key
     */
    decryptMessage: (ciphertext, privateKeyPem) => {
        if (!privateKeyPem) return false;
        const decryptor = new window.JSEncrypt();
        decryptor.setPrivateKey(privateKeyPem);
        return decryptor.decrypt(ciphertext);
    }
};
