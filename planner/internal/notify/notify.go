// Package notify 는 웹푸시로 알림을 보낸다. 판정은 store.NotifyDue.
// iOS 는 홈 화면에 추가한 웹앱(16.4+)에만 푸시가 간다.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/sh5080/home-hub/planner/internal/store"
)

// Sender 는 서명 키를 들고 보낸다.
type Sender struct {
	st  *store.Store
	log *slog.Logger
}

func New(st *store.Store, log *slog.Logger) *Sender { return &Sender{st: st, log: log} }

// Keys 는 VAPID 키. 없으면 만들어 DB 에 둔다 — 바뀌면 모든 구독이 무효가 된다.
func (s *Sender) Keys(ctx context.Context) (pub, priv string, err error) {
	pub, ok1, err := s.st.KVGet(ctx, "vapid_public")
	if err != nil {
		return "", "", err
	}
	priv, ok2, err := s.st.KVGet(ctx, "vapid_private")
	if err != nil {
		return "", "", err
	}
	if ok1 && ok2 {
		return pub, priv, nil
	}
	priv, pub, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	if err := s.st.KVSet(ctx, "vapid_private", priv); err != nil {
		return "", "", err
	}
	if err := s.st.KVSet(ctx, "vapid_public", pub); err != nil {
		return "", "", err
	}
	return pub, priv, nil
}

// Payload 는 서비스 워커가 받는 내용이다.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// ErrNoDevice 는 받을 기기가 하나도 없을 때다.
var ErrNoDevice = errors.New("알림을 켠 기기가 없어요")

// Send 는 users 의 모든 기기로 보낸다(비면 가족 전체). 성공한 기기 수를 준다.
// 404/410 이면 구독을 지운다.
func (s *Sender) Send(ctx context.Context, users []int64, p Payload) (int, error) {
	pub, priv, err := s.Keys(ctx)
	if err != nil {
		return 0, err
	}
	subject, _, _ := s.st.KVGet(ctx, "push_subject")
	if subject == "" {
		subject = "mailto:planner@example.com"
	}
	subs, err := s.st.PushSubs(ctx, users)
	if err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, ErrNoDevice
	}
	body, _ := json.Marshal(p)
	ok := 0
	var lastErr error
	for _, sub := range subs {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		res, err := webpush.SendNotificationWithContext(cctx, body, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}, &webpush.Options{
			Subscriber:      subject,
			VAPIDPublicKey:  pub,
			VAPIDPrivateKey: priv,
			TTL:             3600,
			Urgency:         webpush.UrgencyHigh,
			Topic:           topic(p.Tag),
		})
		cancel()
		if err != nil {
			lastErr = err
			s.log.Warn("push send", "err", err, "user", sub.UserID, "device", sub.Device)
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
			_ = s.st.PushUnsubscribe(ctx, sub.Endpoint)
			s.log.Info("push sub gone, removed", "user", sub.UserID, "device", sub.Device, "status", res.StatusCode, "body", string(msg))
		case res.StatusCode >= 300:
			lastErr = fmt.Errorf("push %d: %s", res.StatusCode, msg)
			s.log.Warn("push rejected", "status", res.StatusCode, "body", string(msg), "user", sub.UserID, "device", sub.Device)
		default:
			ok++
			s.st.PushTouch(ctx, sub.Endpoint)
		}
	}
	if ok == 0 && lastErr != nil {
		return 0, lastErr
	}
	return ok, nil
}

// topic 은 같은 종류 알림을 덮어쓰게 한다. URL-safe base64 문자 32자 이하.
func topic(tag string) string {
	out := make([]byte, 0, 32)
	for i := 0; i < len(tag) && len(out) < 32; i++ {
		c := tag[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			out = append(out, c)
		}
	}
	return string(out)
}

// Run 은 1분마다 알림을 확인한다. ctx 가 끝나면 멈춘다.
func (s *Sender) Run(ctx context.Context) {
	// 분이 바뀐 직후에 돌게 맞춘다.
	wait := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute + 2*time.Second))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.tick(ctx)
		timer.Reset(time.Until(time.Now().Truncate(time.Minute).Add(time.Minute + 2*time.Second)))
	}
}

func (s *Sender) tick(ctx context.Context) {
	due, err := s.st.NotifyDue(ctx, time.Now())
	if err != nil {
		s.log.Warn("notify due", "err", err)
		return
	}
	for _, n := range due {
		// 먼저 적고 보낸다 — 두 번 가는 것보다 한 번 빠지는 게 낫다.
		if err := s.st.NotifyMarkSent(ctx, n.RuleID, n.Key); err != nil {
			s.log.Warn("notify mark", "err", err)
			continue
		}
		cnt, err := s.Send(ctx, n.Recipients, Payload{Title: n.Title, Body: n.Body, URL: n.URL, Tag: fmt.Sprintf("rule-%d", n.RuleID)})
		if err != nil && !errors.Is(err, ErrNoDevice) {
			s.log.Warn("notify send", "rule", n.RuleID, "err", err)
			continue
		}
		s.log.Info("notify sent", "rule", n.RuleID, "key", n.Key, "devices", cnt)
	}
}
