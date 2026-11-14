-- [plus] 上游调度台：存档题作品的预览链接签名密钥（iframe 带不了 Authorization 头，预览接口靠 HMAC 签名鉴权）。
-- 不用 srcdoc：srcdoc 会继承站点的 CSP（只允许带 nonce 的脚本），作品里的动画跑不起来。

ALTER TABLE plus_dispatch_config ADD COLUMN IF NOT EXISTS preview_secret TEXT NOT NULL DEFAULT '';

UPDATE plus_dispatch_config
SET preview_secret = replace(gen_random_uuid()::text || gen_random_uuid()::text, '-', '')
WHERE preview_secret = '';

