package onboarding

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// RateScopeSuggestIcon caps the icon suggestions per user: the forms ask while
// the user types, and each answer not already cached is a paid AI round trip.
const RateScopeSuggestIcon = "onboarding-icon"

// The form waits on the answer, so it gets far less time than the background
// jobs; a late answer is no answer.
const (
	suggestIconTimeout   = 8 * time.Second
	suggestIconCacheSize = 2048
)

var suggestIconSystem = `You pick the icon for an item a user is naming in their personal budget app. ` +
	`A category is what money is spent on (type expense) or comes from (type income); ` +
	`an account is where money is kept (a card, a bank account, cash, savings, an investment). ` + iconRules +
	`Reply with JSON only, exactly {"icon":"<icon>"}; use "" when no icon fits.`

type suggestIconFacts struct {
	Timezone string `json:"timezone"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Currency string `json:"currency,omitempty"`
}

// SuggestIcon asks the AI for the catalogue icon that fits a name the user is
// typing. Every way it can come to nothing (no AI configured, over the per-user
// limit, a failed, late or unusable reply) answers "" rather than an error, so
// typing never shows one. Answers are cached per user; the name is never logged.
func (s *Service) SuggestIcon(ctx context.Context, userID vo.Id, req model.OnboardingSuggestIconRequest) (*model.OnboardingSuggestIconResult, error) {
	out := &model.OnboardingSuggestIconResult{}
	reqctx.AddLogAttr(ctx, "kind", req.Kind)
	if s.completer == nil {
		reqctx.AddLogAttr(ctx, "outcome", "no_ai")
		return out, nil
	}
	f := s.promptContext(ctx, userID)
	facts := suggestIconFacts{
		Timezone: f.Timezone,
		Language: f.Language,
		Kind:     req.Kind,
		Name:     strings.TrimSpace(req.Name),
		Type:     req.Type,
		Currency: strings.ToUpper(req.Currency),
	}
	key := strings.Join([]string{userID.String(), facts.Language, facts.Kind, facts.Type, facts.Currency, strings.ToLower(facts.Name)}, "\x00")
	if icon, ok := s.iconCache.get(key); ok {
		reqctx.AddLogAttr(ctx, "outcome", "cached")
		out.Icon = icon
		return out, nil
	}
	if s.limiter != nil {
		if s.limiter.Allow(ctx, RateScopeSuggestIcon, userID.String()) != nil || s.limiter.Reserve(RateScopeSuggestIcon, userID.String()) != nil {
			reqctx.AddLogAttr(ctx, "outcome", "rate_limited")
			return out, nil
		}
	}
	// The form drops a request as soon as the name changes; the call it paid for
	// still finishes, so its answer is cached for the next time that name is asked.
	icon, err := s.askIcon(context.WithoutCancel(ctx), userID, facts)
	if err != nil {
		reqctx.AddLogAttr(ctx, "outcome", "failed")
		if !errors.Is(err, context.Canceled) {
			// Never log the prompt, the reply or the error text: they can carry the name.
			slog.WarnContext(ctx, "onboarding icon suggestion failed", "user_id", userID.String(), "kind", req.Kind, "err_type", fmt.Sprintf("%T", err))
		}
		return out, nil
	}
	s.iconCache.put(key, icon)
	if icon == "" {
		reqctx.AddLogAttr(ctx, "outcome", "none")
	} else {
		reqctx.AddLogAttr(ctx, "outcome", "suggested")
	}
	out.Icon = icon
	return out, nil
}

// askIcon reads {"icon":"..."} out of the reply (tolerating a code fence or
// prose around it); an icon outside the catalogue is no suggestion.
func (s *Service) askIcon(ctx context.Context, userID vo.Id, facts suggestIconFacts) (string, error) {
	body, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	callCtx, cancel := context.WithTimeout(ctx, suggestIconTimeout)
	defer cancel()
	raw, err := s.completer.Complete(callCtx, userID.String(), suggestIconSystem, string(body))
	if err != nil {
		return "", err
	}
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return "", MalformedReplyError{}
	}
	var parsed struct {
		Icon string `json:"icon"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return "", MalformedReplyError{}
	}
	icon := strings.TrimSpace(parsed.Icon)
	if !iconSet[icon] {
		return "", nil
	}
	return icon, nil
}

// iconCache is a small LRU of answered suggestions, so a user retyping a name
// (or the SPA asking again after a reload) costs no second round trip.
type iconCache struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[string]*list.Element
}

type iconCacheEntry struct{ key, icon string }

func newIconCache(max int) *iconCache {
	return &iconCache{max: max, order: list.New(), items: map[string]*list.Element{}}
}

func (c *iconCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(el)
	return el.Value.(*iconCacheEntry).icon, true
}

func (c *iconCache) put(key, icon string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*iconCacheEntry).icon = icon
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&iconCacheEntry{key: key, icon: icon})
	if c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*iconCacheEntry).key)
	}
}

// ApplySuggestedIcon gives the caller's category a catalogue icon the client
// was suggested after creating it, through the category feature's conditional
// icon write: only while the category still shows ExpectedIcon, so an icon the
// user changed meanwhile wins, and never touching the name.
func (s *Service) ApplySuggestedIcon(ctx context.Context, userID vo.Id, req model.OnboardingApplySuggestedIconRequest) (*model.OnboardingApplySuggestedIconResult, error) {
	icon := strings.TrimSpace(req.Icon)
	if !iconSet[icon] {
		return nil, invalidChoice("icon")
	}
	id := vo.MustParseId(strings.TrimSpace(req.CategoryId))
	own, err := s.steps.Categories.OwnCategories(ctx, userID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, c := range own {
		if strings.EqualFold(c.Id, id.String()) {
			found = true
			break
		}
	}
	if !found {
		return nil, errs.NewNotFound("Category not found")
	}
	applied, err := s.steps.Categories.ReplaceIcon(ctx, userID, id, strings.TrimSpace(req.ExpectedIcon), icon)
	if err != nil {
		return nil, err
	}
	reqctx.AddLogAttr(ctx, "category_id", id.String())
	return &model.OnboardingApplySuggestedIconResult{Applied: applied}, nil
}
