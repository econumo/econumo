// Package onboarding holds the Day 0 setup writes the SPA's first-run flow
// makes. Each write answers at once; when an AI provider is configured, a
// background job then asks it which of the new accounts are savings accounts
// (and, after an import, which icons fit the categories and accounts) and
// applies the answer.
package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// RateScopeClassify caps the background AI classification per user: each job
// is a paid round trip. Over the cap the write still succeeds, unclassified.
const RateScopeClassify = "onboarding-ai"

// RateScopeIcons caps the icon reviews after CSV imports on their own, so a
// run of imports never uses up the savings job's allowance.
const RateScopeIcons = "import-ai-icons"

// iconReviewQuiet is how long after an import's last chunk its icon review
// starts: a large file arrives in several requests and is reviewed once.
const iconReviewQuiet = 15 * time.Second

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

// Scheduler runs f once after d unless the returned stop is called first.
// Production uses a real timer; tests fire the callbacks themselves.
type Scheduler func(d time.Duration, f func()) (stop func() bool)

// AfterFunc is the production Scheduler.
func AfterFunc(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }

type Service struct {
	budgets   BudgetWriter
	accounts  AccountReader
	users     UserFacts
	completer Completer
	limiter   AttemptLimiter
	run       Runner
	schedule  Scheduler
	steps     Steps

	iconsMu      sync.Mutex
	pendingIcons map[string]*pendingIcons
}

func NewService(budgets BudgetWriter, accounts AccountReader, users UserFacts) *Service {
	return &Service{budgets: budgets, accounts: accounts, users: users, run: GoRunner, schedule: AfterFunc, pendingIcons: map[string]*pendingIcons{}}
}

// SetAdvisor enables the background savings classification. A nil completer
// leaves every onboarding write synchronous-only.
func (s *Service) SetAdvisor(c Completer, limiter AttemptLimiter) {
	s.completer, s.limiter = c, limiter
}

// SetSteps wires the dependencies of the three step writes.
func (s *Service) SetSteps(steps Steps) { s.steps = steps }

// SetRunner replaces how background jobs start (tests run them inline).
func (s *Service) SetRunner(r Runner) { s.run = r }

// SetScheduler replaces the timer behind the icon review's quiet period.
func (s *Service) SetScheduler(sc Scheduler) { s.schedule = sc }

// scheduleClassify starts the classification of accountIDs for budgetID, unless AI is
// off or the user is over the cap.
func (s *Service) scheduleClassify(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id) {
	if len(accountIDs) == 0 {
		return
	}
	s.startJob(ctx, userID, jobClassify, func(jobCtx context.Context, facts promptFacts) error {
		return s.classify(jobCtx, userID, budgetID, accountIDs, facts)
	})
}

// jobSpec is a background job's time budget, cap scope (RateScopeClassify
// when empty) and static log messages; part names the request of a job split
// in several (logged as "part").
type jobSpec struct {
	timeout                   time.Duration
	scope                     string
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
	jobIcons = jobSpec{
		timeout:  reviewTimeout,
		scope:    RateScopeIcons,
		skipped:  "import icon review skipped",
		panicked: "import icon review panicked",
		failed:   "import icon review failed",
	}
)

// startJob runs one background AI job for the user, unless AI is off or the
// user is over the cap (every job counts against it).
func (s *Service) startJob(ctx context.Context, userID vo.Id, logs jobSpec, job func(context.Context, promptFacts) error) {
	if !s.reserveJob(ctx, userID, logs) {
		return
	}
	s.runJob(userID, logs, s.promptContext(ctx, userID), job)
}

// reserveJob reports whether a job may start: AI is configured, the user has
// not opted out of it and is under the cap, which this call counts against.
// An unreadable preference starts nothing.
func (s *Service) reserveJob(ctx context.Context, userID vo.Id, logs jobSpec) bool {
	ok, _ := s.reserve(ctx, userID, logs)
	return ok
}

// reserve is reserveJob telling apart why nothing may start: a non-nil error
// is a failure to find out (an unreadable preference, a limiter fault), not
// an opt-out or a full cap.
func (s *Service) reserve(ctx context.Context, userID vo.Id, logs jobSpec) (bool, error) {
	if s.completer == nil {
		return false, nil
	}
	if s.users != nil {
		off, err := s.users.AIOptedOut(ctx, userID)
		if err != nil || off {
			return false, err
		}
	}
	if s.limiter != nil {
		scope := logs.scope
		if scope == "" {
			scope = RateScopeClassify
		}
		if err := s.limiter.Allow(ctx, scope, userID.String()); err != nil {
			if _, full := errs.AsTooManyRequests(err); !full {
				return false, err
			}
			slog.InfoContext(ctx, logs.skipped, "user_id", userID.String(), "reason", "rate_limited")
			return false, nil
		}
		if err := s.limiter.Reserve(scope, userID.String()); err != nil {
			if _, full := errs.AsTooManyRequests(err); !full {
				return false, err
			}
			return false, nil
		}
	}
	return true, nil
}

// mayAsk re-reads the preference right before a provider call: the user may
// have opted out since the job was scheduled. Unreadable asks nothing.
func (s *Service) mayAsk(ctx context.Context, userID vo.Id) bool {
	if s.users == nil {
		return true
	}
	off, err := s.users.AIOptedOut(ctx, userID)
	return err == nil && !off
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
