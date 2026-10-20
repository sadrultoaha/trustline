package compliance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trustline/trustline/internal/activity"
	"github.com/trustline/trustline/internal/catalog"
	"github.com/trustline/trustline/internal/db"
	"github.com/trustline/trustline/internal/domain"
)

type TrainingStatus struct {
	Course      *catalog.TrainingCourse `json:"course"`
	Completed   bool                    `json:"completed"`
	LastScore   *int                    `json:"last_score,omitempty"`
	LastPassed  *bool                   `json:"last_passed,omitempty"`
	CompletedAt *time.Time              `json:"completed_at,omitempty"`
	ExpiresAt   *time.Time              `json:"expires_at,omitempty"`
}

func (s *Service) course() *catalog.TrainingCourse {
	if len(s.Catalog.Training) == 0 {
		return nil
	}
	return &s.Catalog.Training[0]
}

func (s *Service) TrainingFor(ctx context.Context, orgID, userID domain.ID) (*TrainingStatus, error) {
	c := s.course()
	if c == nil {
		return nil, domain.ErrNotFound
	}
	st := &TrainingStatus{Course: c}
	var score int
	var passed bool
	var at time.Time
	err := s.Pool.QueryRow(ctx, `SELECT score, passed, completed_at FROM training_completions WHERE org_id = $1 AND user_id = $2 AND course_id = $3 ORDER BY completed_at DESC LIMIT 1`, orgID, userID, c.ID).Scan(&score, &passed, &at)
	if db.IsNoRows(err) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.LastScore, st.LastPassed = &score, &passed
	var best time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT COALESCE(max(completed_at), 'epoch') FROM training_completions WHERE org_id = $1 AND user_id = $2 AND course_id = $3 AND passed`, orgID, userID, c.ID).Scan(&best); err != nil {
		return nil, err
	}
	if best.After(time.Unix(0, 0)) {
		exp := best.Add(time.Duration(c.ValidDays) * 24 * time.Hour)
		st.CompletedAt, st.ExpiresAt = &best, &exp
		st.Completed = exp.After(time.Now())
	}
	return st, nil
}

type QuizResult struct {
	Score    int                  `json:"score"`
	Passed   bool                 `json:"passed"`
	Required int                  `json:"required"`
	Feedback []QuizAnswerFeedback `json:"feedback"`
}

type QuizAnswerFeedback struct {
	QuestionID  string `json:"question_id"`
	Correct     bool   `json:"correct"`
	Answer      int    `json:"correct_answer"`
	Explanation string `json:"explanation"`
}

// SubmitQuiz grades the quiz and records the completion.
func (s *Service) SubmitQuiz(ctx context.Context, orgID, userID domain.ID, answers map[string]int, actor domain.Actor) (*QuizResult, error) {
	c := s.course()
	if c == nil {
		return nil, domain.ErrNotFound
	}
	if len(answers) < len(c.Questions) {
		return nil, domain.Invalid("answers", "answer every question before submitting")
	}
	res := &QuizResult{Required: c.PassingScore}
	correct := 0
	for _, q := range c.Questions {
		got, ok := answers[q.ID]
		fb := QuizAnswerFeedback{QuestionID: q.ID, Correct: ok && got == q.Answer, Answer: q.Answer, Explanation: q.Explanation}
		if fb.Correct {
			correct++
		}
		res.Feedback = append(res.Feedback, fb)
	}
	res.Score = correct * 100 / len(c.Questions)
	res.Passed = res.Score >= c.PassingScore
	raw, _ := json.Marshal(answers)
	err := db.Tx(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO training_completions (id, org_id, user_id, course_id, score, passed, answers) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			domain.NewID(), orgID, userID, c.ID, res.Score, res.Passed, raw); err != nil {
			return err
		}
		if err := activity.Record(ctx, tx, orgID, actor, activity.Entry{Action: "training.completed", TargetType: "training", TargetID: c.ID, TargetLabel: c.Title,
			Metadata: map[string]any{"score": res.Score, "passed": res.Passed}}); err != nil {
			return err
		}
		if res.Passed {
			if err := s.closeTrainingTasks(ctx, tx, orgID, userID); err != nil {
				return err
			}
		}
		return s.touchPlatform(ctx, tx, orgID)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) closeTrainingTasks(ctx context.Context, tx pgx.Tx, orgID, userID domain.ID) error {
	_, err := tx.Exec(ctx, `UPDATE tasks t SET status = 'done', resolution = 'Training passed.', completed_at = now(), updated_at = now()
		FROM people p WHERE t.org_id = $1 AND t.person_id = p.id AND p.user_id = $2 AND t.status = 'open' AND t.item_key = 'training'`, orgID, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tasks t SET status = 'done', resolution = 'Training passed.', completed_at = now(), updated_at = now()
		FROM org_memberships m WHERE t.org_id = $1 AND t.kind = 'training_renewal' AND t.assignee_membership_id = m.id AND m.user_id = $2 AND t.status = 'open'`, orgID, userID)
	return err
}
