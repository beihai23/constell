package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	pbv1 "github.com/constell/constell/backend/pkg/proto/auth/v1"
)

// --- Mock Repository ---

type mockRepo struct {
	users        map[string]*User // email -> User
	createErr    error
	lastHashedPw string
}

// Verify mockRepo satisfies UserRepository at compile time.
var _ UserRepository = (*mockRepo)(nil)

func newMockRepo() *mockRepo {
	return &mockRepo{users: make(map[string]*User)}
}

func (m *mockRepo) CreateUser(ctx context.Context, nickname, email, hashedPassword string) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	id := fmt.Sprintf("user-%s", email)
	m.users[email] = &User{
		ID: id, Email: email, PasswordHash: hashedPassword, Nickname: nickname,
	}
	m.lastHashedPw = hashedPassword
	return id, nil
}

func (m *mockRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

// --- Tests ---

func TestHashAndCheckPassword(t *testing.T) {
	password := "supersecret123"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == password {
		t.Fatal("hash should not equal plain password")
	}
	if !checkPassword(password, hash) {
		t.Fatal("checkPassword should return true for correct password")
	}
	if checkPassword("wrongpassword", hash) {
		t.Fatal("checkPassword should return false for wrong password")
	}
	t.Log("hashPassword and checkPassword working correctly")
}

func TestHashToken(t *testing.T) {
	token := "some-refresh-token-value"
	h := hashToken(token)

	expected := sha256.Sum256([]byte(token))
	expectedHex := hex.EncodeToString(expected[:])
	if h != expectedHex {
		t.Fatalf("expected %q, got %q", expectedHex, h)
	}

	h2 := hashToken(token)
	if h != h2 {
		t.Fatal("hashToken should be deterministic")
	}

	h3 := hashToken("different-token")
	if h == h3 {
		t.Fatal("different tokens should produce different hashes")
	}
	t.Log("hashToken working correctly")
}

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		nickname string
		wantCode connect.Code
	}{
		{
			name: "missing email", email: "", password: "password123",
			nickname: "alice", wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing password", email: "alice@example.com", password: "",
			nickname: "alice", wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing nickname", email: "alice@example.com",
			password: "password123", nickname: "", wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "short password", email: "alice@example.com",
			password: "short", nickname: "alice", wantCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &AuthService{
				repo: newMockRepo(), rdb: nil, jwtSecret: "test-secret",
			}
			req := connect.NewRequest(&pbv1.RegisterRequest{
				Email: tt.email, Password: tt.password, Nickname: tt.nickname,
			})
			_, err := svc.Register(context.Background(), req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			connErr, ok := err.(*connect.Error)
			if !ok {
				t.Fatalf("expected *connect.Error, got %T", err)
			}
			if connErr.Code() != tt.wantCode {
				t.Fatalf("expected code %v, got %v (message: %s)",
					tt.wantCode, connErr.Code(), connErr.Message())
			}
			t.Logf("%s: correctly rejected with code %v", tt.name, connErr.Code())
		})
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	repo := newMockRepo()
	repo.users["alice@example.com"] = &User{
		ID: "user-existing", Email: "alice@example.com",
		PasswordHash: "$2a$10$somehash", Nickname: "alice",
	}
	svc := &AuthService{repo: repo, rdb: nil, jwtSecret: "test-secret"}
	req := connect.NewRequest(&pbv1.RegisterRequest{
		Email: "alice@example.com", Password: "password123", Nickname: "alice",
	})
	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeAlreadyExists {
		t.Fatalf("expected CodeAlreadyExists, got %v", connErr.Code())
	}
	t.Log("duplicate email correctly rejected with CodeAlreadyExists")
}

func TestLoginUserNotFound(t *testing.T) {
	svc := &AuthService{repo: newMockRepo(), rdb: nil, jwtSecret: "test-secret"}
	req := connect.NewRequest(&pbv1.LoginRequest{
		Email: "nobody@example.com", Password: "password123",
	})
	_, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unknown email, got nil")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", connErr.Code())
	}
	t.Log("unknown email correctly rejected with CodeUnauthenticated")
}

func TestLoginWrongPassword(t *testing.T) {
	repo := newMockRepo()
	hash, _ := hashPassword("correctpassword")
	repo.users["alice@example.com"] = &User{
		ID: "user-alice", Email: "alice@example.com",
		PasswordHash: hash, Nickname: "alice",
	}
	svc := &AuthService{repo: repo, rdb: nil, jwtSecret: "test-secret"}
	req := connect.NewRequest(&pbv1.LoginRequest{
		Email: "alice@example.com", Password: "wrongpassword",
	})
	_, err := svc.Login(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected CodeUnauthenticated, got %v", connErr.Code())
	}
	t.Log("wrong password correctly rejected with CodeUnauthenticated")
}

func TestLoginValidation(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		password string
		wantCode connect.Code
	}{
		{
			name: "missing email", email: "", password: "password123",
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing password", email: "alice@example.com", password: "",
			wantCode: connect.CodeInvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &AuthService{repo: newMockRepo(), rdb: nil, jwtSecret: "test-secret"}
			req := connect.NewRequest(&pbv1.LoginRequest{
				Email: tt.email, Password: tt.password,
			})
			_, err := svc.Login(context.Background(), req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			connErr, ok := err.(*connect.Error)
			if !ok {
				t.Fatalf("expected *connect.Error, got %T", err)
			}
			if connErr.Code() != tt.wantCode {
				t.Fatalf("expected code %v, got %v", tt.wantCode, connErr.Code())
			}
		})
	}
}

func TestRefreshTokenValidation(t *testing.T) {
	svc := &AuthService{repo: newMockRepo(), rdb: nil, jwtSecret: "test-secret"}
	req := connect.NewRequest(&pbv1.RefreshTokenRequest{RefreshToken: ""})
	_, err := svc.RefreshToken(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty refresh token, got nil")
	}
	connErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connErr.Code() != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", connErr.Code())
	}
	t.Log("empty refresh token correctly rejected with CodeInvalidArgument")
}
