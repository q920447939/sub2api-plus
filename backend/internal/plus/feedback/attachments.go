// Package feedback — [plus] 附件：只收图片，统一重新编码（去掉 EXIF 等元数据），经鉴权接口读取
package feedback

import (
	"bytes"
	"context"
	"image"
	_ "image/gif" // 注册 gif 解码
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/lib/pq"
	_ "golang.org/x/image/webp" // 注册 webp 解码
)

const maxPixels = 40_000_000 // 防解压炸弹

func pqStrings(s []string) any { return pq.StringArray(s) }

// Upload 保存一张图片，返回附件 ID；之后在提交/补充/回复时绑定到反馈
func (s *Service) Upload(ctx context.Context, uploaderID int64, data []byte) (int64, error) {
	cfg, err := s.Config(ctx)
	if err != nil {
		return 0, err
	}
	if !cfg.Enabled {
		return 0, ErrDisabled
	}
	if cfg.Attachments.MaxCount == 0 {
		return 0, inputErr("暂不支持上传图片")
	}
	if int64(len(data)) > int64(cfg.Attachments.MaxMB)<<20 {
		return 0, inputErr("图片太大")
	}
	out, mime, ext, w, h, err := reencode(data)
	if err != nil {
		return 0, err
	}
	path, err := s.Blobs.Put(ctx, out, ext)
	if err != nil {
		return 0, err
	}
	a := &Attachment{UploaderID: uploaderID, Path: path, Mime: mime, Size: int64(len(out))}
	if err := s.Repo.InsertAttachment(ctx, a, w, h); err != nil {
		_ = s.Blobs.Remove(path)
		return 0, err
	}
	return a.ID, nil
}

// reencode 解码后重新编码：去掉元数据，也顺带校验确实是图片。gif/webp 统一转 png
func reencode(data []byte) (out []byte, mime, ext string, w, h int, err error) {
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return nil, "", "", 0, 0, inputErr("只支持 JPG、PNG、GIF、WebP 图片")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", "", 0, 0, inputErr("图片无法识别")
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, "", "", 0, 0, inputErr("图片尺寸太大")
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", "", 0, 0, inputErr("图片无法识别")
	}
	var buf bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88})
		mime, ext = "image/jpeg", "jpg"
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img)
		mime, ext = "image/png", "png"
	}
	if err != nil {
		return nil, "", "", 0, 0, err
	}
	b := img.Bounds()
	return buf.Bytes(), mime, ext, b.Dx(), b.Dy(), nil
}

// OpenAttachment 读取附件。管理员可读全部；用户只能读自己上传的，或挂在自己反馈上的（管理员回复里的图）
func (s *Service) OpenAttachment(ctx context.Context, viewerID int64, admin bool, id int64) (io.ReadCloser, string, error) {
	a, err := s.Repo.Attachment(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if !admin && a.UploaderID != viewerID {
		if a.FeedbackID == nil {
			return nil, "", ErrNotFound
		}
		f, err := s.Repo.Get(ctx, *a.FeedbackID)
		if err != nil || f.UserID != viewerID {
			return nil, "", ErrNotFound
		}
		// 内部备注里的图不给用户看
		msgs, err := s.Repo.Messages(ctx, f.ID, true)
		if err != nil {
			return nil, "", err
		}
		for _, m := range msgs {
			if m.Internal && containsID(m.Attachments, id) {
				return nil, "", ErrNotFound
			}
		}
	}
	rc, err := s.Blobs.Open(a.Path)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, a.Mime, nil
}

func containsID(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// CleanOrphans 删除上传后 24 小时仍未绑定的附件
func (s *Service) CleanOrphans(ctx context.Context) {
	list, err := s.Repo.OrphanAttachments(ctx, s.Now().Add(-24*time.Hour), 200)
	if err != nil {
		log.Printf("[plus.feedback] orphan attachments: %v", err)
		return
	}
	for _, a := range list {
		if err := s.Blobs.Remove(a.Path); err != nil {
			log.Printf("[plus.feedback] remove %s: %v", a.Path, err)
			continue
		}
		_ = s.Repo.DeleteAttachment(ctx, a.ID)
	}
}
