package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type sessionCache struct {
	userID    int
	expiresAt time.Time
}

type SessionRepository struct {
	db    DBTX
	cache map[string]sessionCache
	mutex sync.RWMutex
}

func NewSessionRepository(db DBTX) *SessionRepository {
	return &SessionRepository{
		db:    db,
		cache: make(map[string]sessionCache),
	}
}

// セッションを作成し、セッションIDと有効期限を返す
func (r *SessionRepository) Create(ctx context.Context, userBusinessID int, duration time.Duration) (string, time.Time, error) {
	sessionUUID, err := uuid.NewRandom()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(duration)
	sessionIDStr := sessionUUID.String()

	query := "INSERT INTO user_sessions (session_uuid, user_id, expires_at) VALUES (?, ?, ?)"
	_, err = r.db.ExecContext(ctx, query, sessionIDStr, userBusinessID, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}

	// キャッシュに保存
	r.mutex.Lock()
	r.cache[sessionIDStr] = sessionCache{
		userID:    userBusinessID,
		expiresAt: expiresAt,
	}
	r.mutex.Unlock()

	return sessionIDStr, expiresAt, nil
}

// セッションIDからユーザーIDを取得（キャッシュ優先）
func (r *SessionRepository) FindUserBySessionID(ctx context.Context, sessionID string) (int, error) {
	// まずキャッシュをチェック
	r.mutex.RLock()
	cached, exists := r.cache[sessionID]
	r.mutex.RUnlock()

	if exists {
		// キャッシュが有効かチェック
		if time.Now().Before(cached.expiresAt) {
			return cached.userID, nil
		}
		// 期限切れの場合はキャッシュから削除
		r.mutex.Lock()
		delete(r.cache, sessionID)
		r.mutex.Unlock()
	}

	// キャッシュにない場合はDBから取得（1回のクエリで両方を取得）
	var sessionData struct {
		UserID    int       `db:"user_id"`
		ExpiresAt time.Time `db:"expires_at"`
	}
	query := `
		SELECT 
			u.user_id,
			s.expires_at
		FROM users u
		JOIN user_sessions s ON u.user_id = s.user_id
		WHERE s.session_uuid = ? AND s.expires_at > ?`
	err := r.db.GetContext(ctx, &sessionData, query, sessionID, time.Now())
	if err != nil {
		return 0, err
	}

	// DBから取得したセッション情報をキャッシュに保存
	r.mutex.Lock()
	r.cache[sessionID] = sessionCache{
		userID:    sessionData.UserID,
		expiresAt: sessionData.ExpiresAt,
	}
	r.mutex.Unlock()

	return sessionData.UserID, nil
}
