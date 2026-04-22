ALTER TABLE messages DROP key_for_sender;
ALTER TABLE messages DROP key_for_receiver;
ALTER TABLE messages ADD encrypted_aes_key text;
