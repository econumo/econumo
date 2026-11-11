// Package onboarding holds the Day 0 setup writes the SPA's first-run flow
// makes. Each write answers at once; when an AI provider is configured, a
// background job then asks it which icons fit the categories and accounts
// still on a default icon, which of the new accounts are savings accounts and
// which categories repay a loan (creating a debt for each), and applies the
// answer.
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

// classifyTimeout bounds the savings-only job; reviewTimeout the import review,
// whose reply (an icon per category and account, chosen from the whole
// catalogue) takes real providers far longer. Both run in the background.
const (
	classifyTimeout = 15 * time.Second
	reviewTimeout   = 90 * time.Second
)

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
	steps     Steps
	debts     DebtCreator
}

func NewService(budgets BudgetWriter, accounts AccountReader, timezones TimezoneReader) *Service {
	return &Service{budgets: budgets, accounts: accounts, timezones: timezones, run: GoRunner}
}

// SetAdvisor enables the background savings classification. A nil completer
// leaves every onboarding write synchronous-only.
func (s *Service) SetAdvisor(c Completer, limiter AttemptLimiter) {
	s.completer, s.limiter = c, limiter
}

// SetSteps wires the dependencies of the three step writes.
func (s *Service) SetSteps(steps Steps) { s.steps = steps }

// SetDebts enables the debt detection; the debt service is built after this
// one, so it cannot be a constructor argument.
func (s *Service) SetDebts(d DebtCreator) { s.debts = d }

// SetRunner replaces how background jobs start (tests run them inline).
func (s *Service) SetRunner(r Runner) { s.run = r }

// jobSpec is a background job's time budget and static log messages; part
// names the request of a job split in several (logged as "part").
type jobSpec struct {
	timeout                   time.Duration
	skipped, panicked, failed string
	part                      string
}

var (
	jobClassify = jobSpec{
		timeout:  classifyTimeout,
		skipped:  "onboarding savings classification skipped",
		panicked: "onboarding savings classification panicked",
		failed:   "onboarding savings classification failed",
	}
	jobReview = jobSpec{
		timeout:  reviewTimeout,
		skipped:  "onboarding import review skipped",
		panicked: "onboarding import review panicked",
		failed:   "onboarding import review failed",
	}
	jobStepReview = jobSpec{
		timeout:  reviewTimeout,
		skipped:  "onboarding step review skipped",
		panicked: "onboarding step review panicked",
		failed:   "onboarding step review failed",
	}
	jobDebts = jobSpec{
		timeout:  reviewTimeout,
		skipped:  "onboarding debt detection skipped",
		panicked: "onboarding debt detection panicked",
		failed:   "onboarding debt detection failed",
	}
)

// reserveJob reports whether a job may start: AI is configured and the user
// is under the cap, which this call counts against.
func (s *Service) reserveJob(ctx context.Context, userID vo.Id, logs jobSpec) bool {
	if s.completer == nil {
		return false
	}
	if s.limiter != nil {
		if err := s.limiter.Allow(ctx, RateScopeClassify, userID.String()); err != nil {
			slog.InfoContext(ctx, logs.skipped, "user_id", userID.String(), "reason", "rate_limited")
			return false
		}
		if err := s.limiter.Reserve(RateScopeClassify, userID.String()); err != nil {
			return false
		}
	}
	return true
}

// runJob starts job through the runner with a fresh context (the request's is
// cancelled the moment the response is written) and its own timeout; a panic
// or an error is logged and goes no further.
func (s *Service) runJob(userID vo.Id, logs jobSpec, facts promptFacts, job func(context.Context, promptFacts) error) {
	attrs := []any{"user_id", userID.String()}
	if logs.part != "" {
		attrs = append(attrs, "part", logs.part)
	}
	s.run(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error(logs.panicked, append(attrs, "err_type", fmt.Sprintf("%T", r))...)
			}
		}()
		jobCtx, cancel := context.WithTimeout(context.Background(), logs.timeout)
		defer cancel()
		if err := job(jobCtx, facts); err != nil {
			// Never log the prompt, the reply or the error text: they can carry names.
			slog.Warn(logs.failed, append(attrs, "err_type", fmt.Sprintf("%T", err))...)
		}
	})
}
