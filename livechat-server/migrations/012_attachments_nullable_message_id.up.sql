-- 012: Make attachments.message_id nullable so upload can be initiated
-- before the message is created (the message later references the attachment).
ALTER TABLE attachments ALTER COLUMN message_id DROP NOT NULL;
