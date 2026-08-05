-- Normal/VIP visitor tiering (overview.md §6.9.1): tier lives on the
-- visitor record itself since it's a property of the customer, not the
-- chat; handles_vip lives on user_merchant since "who handles VIP" is
-- scoped per merchant, same as every other agent-merchant relationship.
ALTER TABLE visitor ADD COLUMN IF NOT EXISTS tier ENUM('normal','vip') NOT NULL DEFAULT 'normal';
ALTER TABLE user_merchant ADD COLUMN IF NOT EXISTS handles_vip TINYINT(1) NOT NULL DEFAULT 0;
