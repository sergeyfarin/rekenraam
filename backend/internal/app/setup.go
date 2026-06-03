package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"rekenraam/backend/internal/db"
)

const (
	SetupStepStatusPending   = "pending"
	SetupStepStatusCompleted = "completed"

	InstallStateFresh            = "fresh"
	InstallStateConfigured       = "configured"
	InstallStateRecoveryRequired = "recovery_required"

	argon2MemoryKiB   = 19 * 1024
	argon2Iterations  = 2
	argon2Parallelism = 1
	argon2SaltLength  = 16
	argon2KeyLength   = 32
	sessionTokenBytes = 32
	SessionLifetime   = 30 * 24 * time.Hour

	ownerPasswordMinLength = 12
	ownerPasswordMaxBytes  = 1024

	ownerUsernameMaxBytes = 64
)

var ErrSetupAlreadyComplete = errors.New("setup already complete")

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

type SetupService struct {
	repository *db.SetupRepository
}

type SetupStatus struct {
	Completed        bool
	CurrentStep      string
	Steps            []SetupStep
	InstallState     string
	ImplementedSteps []string
	BlockingStep     string
}

type SetupStep struct {
	Key    string
	Status string
}

type CreateOwnerInput struct {
	Username string
	Password string
}

type CreateOwnerResult struct {
	Owner        Owner
	SetupStatus  SetupStatus
	SessionToken string
}

type Owner struct {
	ID       int64
	Username string
}

func NewSetupService(repository *db.SetupRepository) *SetupService {
	return &SetupService{repository: repository}
}

func (s *SetupService) Status(ctx context.Context) (SetupStatus, error) {
	overview, err := s.repository.ReadSetupOverview(ctx)
	if err != nil {
		return SetupStatus{}, fmt.Errorf("read setup overview: %w", err)
	}

	stepRecords, err := s.repository.ListSetupSteps(ctx)
	if err != nil {
		return SetupStatus{}, fmt.Errorf("read setup status: %w", err)
	}

	status := SetupStatus{
		Completed:        true,
		Steps:            make([]SetupStep, 0, len(stepRecords)),
		ImplementedSteps: []string{"owner", "book", "currencies", "system_accounts"},
	}

	for _, stepRecord := range stepRecords {
		stepStatus := SetupStepStatusPending
		if stepRecord.CompletedAt.Valid {
			stepStatus = SetupStepStatusCompleted
		} else if status.CurrentStep == "" {
			status.CurrentStep = stepRecord.Key
			status.Completed = false
		}

		if stepStatus != SetupStepStatusCompleted {
			status.Completed = false
		}

		status.Steps = append(status.Steps, SetupStep{
			Key:    stepRecord.Key,
			Status: stepStatus,
		})
	}

	if len(stepRecords) == 0 {
		status.Completed = false
	}

	status.InstallState = deriveInstallState(overview)
	if status.InstallState == InstallStateFresh {
		status.BlockingStep = "owner"
	}

	return status, nil
}

func deriveInstallState(overview db.SetupOverview) string {
	switch {
	case !overview.OwnerExists && !overview.OwnerStepCompleted:
		return InstallStateFresh
	case overview.OwnerExists && overview.OwnerStepCompleted:
		return InstallStateConfigured
	default:
		return InstallStateRecoveryRequired
	}
}

func (s *SetupService) CreateOwner(ctx context.Context, input CreateOwnerInput) (CreateOwnerResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return CreateOwnerResult{}, ValidationError{Message: "username is required"}
	}
	if err := validateOwnerUsername(username); err != nil {
		return CreateOwnerResult{}, err
	}
	if err := validateNewOwnerPassword(input.Password); err != nil {
		return CreateOwnerResult{}, err
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return CreateOwnerResult{}, fmt.Errorf("hash owner password: %w", err)
	}

	sessionToken, sessionTokenHash, err := newSessionToken()
	if err != nil {
		return CreateOwnerResult{}, fmt.Errorf("create owner session token: %w", err)
	}

	now := time.Now().UTC()
	createdAt := now.Format(time.RFC3339)
	ownerRecord, err := s.repository.CompleteOwnerSetup(ctx, db.CompleteOwnerSetupParams{
		Username:         username,
		PasswordHash:     passwordHash,
		SessionTokenHash: sessionTokenHash,
		CreatedAt:        createdAt,
		SessionExpiresAt: sessionExpiresAt(now),
	})
	if err != nil {
		if errors.Is(err, db.ErrOwnerExists) {
			return CreateOwnerResult{}, ErrSetupAlreadyComplete
		}

		return CreateOwnerResult{}, fmt.Errorf("persist owner setup: %w", err)
	}

	setupStatus, err := s.Status(ctx)
	if err != nil {
		return CreateOwnerResult{}, fmt.Errorf("reload setup status: %w", err)
	}

	return CreateOwnerResult{
		Owner: Owner{
			ID:       ownerRecord.ID,
			Username: ownerRecord.Username,
		},
		SetupStatus:  setupStatus,
		SessionToken: sessionToken,
	}, nil
}

func validateOwnerUsername(username string) error {
	if len(username) > ownerUsernameMaxBytes {
		return ValidationError{Message: fmt.Sprintf("username must be at most %d bytes", ownerUsernameMaxBytes)}
	}
	for _, r := range username {
		if r < 32 || r == 127 {
			return ValidationError{Message: "username must not contain control characters"}
		}
	}
	return nil
}

func validateNewOwnerPassword(password string) error {
	if password == "" {
		return ValidationError{Message: "password is required"}
	}
	if utf8.RuneCountInString(password) < ownerPasswordMinLength {
		return ValidationError{Message: "password must be at least 12 characters"}
	}
	if len(password) > ownerPasswordMaxBytes {
		return ValidationError{Message: "password must be at most 1024 bytes"}
	}

	return nil
}

func validateLoginPassword(password string) error {
	if password == "" {
		return ValidationError{Message: "password is required"}
	}
	if len(password) > ownerPasswordMaxBytes {
		return ValidationError{Message: "password must be at most 1024 bytes"}
	}

	return nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate argon2 salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Iterations, argon2MemoryKiB, argon2Parallelism, argon2KeyLength)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2MemoryKiB,
		argon2Iterations,
		argon2Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func newSessionToken() (string, string, error) {
	tokenBytes := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate session token bytes: %w", err)
	}

	plainToken := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(plainToken))

	return plainToken, hex.EncodeToString(tokenHash[:]), nil
}

func sessionExpiresAt(now time.Time) string {
	return now.Add(SessionLifetime).UTC().Format(time.RFC3339)
}
