-- Move from single wrapped key (receiver-only) to dual wrapped keys.
-- This fixes "self-decryption failure" for senders when loading history.
ALTER TABLE messages DROP encrypted_aes_key;
ALTER TABLE messages ADD key_for_sender text;
ALTER TABLE messages ADD key_for_receiver text;
