// Package onboarding holds the Day 0 setup writes the SPA's first-run flow
// makes. Each write answers at once; when an AI provider is configured, a
// background job then asks it which of the new accounts are savings accounts
// and flags them in the budget.
package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// RateScopeClassify caps the background AI classification per user: each job
// is a paid round trip. Over the cap the write still succeeds, unclassified.
const RateScopeClassify = "onboarding-ai"

const classifyTimeout = 15 * time.Second

// Runner starts a background job. Production runs it on a goroutine; tests
// run it inline.
type Runner func(job func())

// GoRunner runs each job on its own goroutine.
func GoRunner(job func()) { go job() }

type Service struct {
	budgets   BudgetWriter
	accounts  AccountReader
	timezones TimezoneReader
	completer Completer
	limiter   AttemptLimiter
	run       Runner
}

func NewService(budgets BudgetWriter, accounts AccountReader, timezones TimezoneReader) *Service {
	return &Service{budgets: budgets, accounts: accounts, timezones: timezones, run: GoRunner}
}

// SetAdvisor enables the background savings classification. A nil completer
// leaves every onboarding write synchronous-only.
func (s *Service) SetAdvisor(c Completer, limiter AttemptLimiter) {
	s.completer, s.limiter = c, limiter
}

// SetRunner replaces how background jobs start (tests run them inline).
func (s *Service) SetRunner(r Runner) { s.run = r }

// schedule starts the classification of accountIDs for budgetID, unless AI is
// off or the user is over the cap. The job gets a fresh context: the request's
// is cancelled the moment the response is written.
func (s *Service) schedule(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id) {
	if s.completer == nil || len(accountIDs) == 0 {
		return
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, RateScopeClassify, userID.String()); err != nil {
			slog.InfoContext(ctx, "onboarding savings classification skipped", "user_id", userID.String(), "reason", "rate_limited")
			return
		}
		if err := s.limiter.Reserve(RateScopeClassify, userID.String()); err != nil {
			return
		}
	}
	facts := s.promptContext(ctx, userID)
	s.run(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("onboarding savings classification panicked", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", r))
			}
		}()
		jobCtx, cancel := context.WithTimeout(context.Background(), classifyTimeout)
		defer cancel()
		if err := s.classify(jobCtx, userID, budgetID, accountIDs, facts); err != nil {
			// Never log the prompt, the reply or the error text: they can carry account names.
			slog.Warn("onboarding savings classification failed", "user_id", userID.String(), "err_type", fmt.Sprintf("%T", err))
		}
	})
}
