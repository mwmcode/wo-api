package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type password struct {
	plainText *string
	hash      []byte
}

func (p *password) Set(textPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(textPassword), 12)
	if err != nil {
		return err
	}

	p.plainText = &textPassword
	p.hash = hash

	return nil
}

func (p *password) Matches(textPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(textPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}
	return true, nil
}

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash password  `json:"-"`
	Bio          string    `json:"bio"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

var AnonymousUser = &User{}

func (u *User) IsAnonymous() bool {
	return u == AnonymousUser
}

type PostgresUserStore struct {
	db *sql.DB
}

func NewPostgresUserStore(db *sql.DB) *PostgresUserStore {
	return &PostgresUserStore{
		db: db,
	}
}

type UserStore interface {
	CreateUser(*User) error
	GetUserByUsername(username string) (*User, error)
	UpdateUser(*User) error
	GetUserByToken(scope, tokenPlainText string) (*User, error)
}

func (pgStore *PostgresUserStore) CreateUser(user *User) error {
	query := `
	INSERT INTO users (username, email, password_hash, bio)
  VALUES ($1, $2, $3, $4)
  RETURNING id, created_at, updated_at`

	err := pgStore.db.QueryRow(query,
		user.Username,
		user.Email,
		user.PasswordHash.hash,
		user.Bio,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return err
	}
	return nil
}

func (pgStore *PostgresUserStore) GetUserByUsername(username string) (*User, error) {
	user := &User{
		PasswordHash: password{},
	}

	query := `
  SELECT id, username, email, password_hash, bio, created_at, updated_at
  FROM users
  WHERE username = $1
  `

	err := pgStore.db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash.hash,
		&user.Bio,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (pgStore *PostgresUserStore) UpdateUser(user *User) error {
	query := `
  UPDATE users
  SET username = $1, email = $2, bio = $3, updated_at = CURRENT_TIMESTAMP
  WHERE id = $4
  RETURNING updated_at`

	result, err := pgStore.db.Exec(query, user.Username, user.Email, user.Bio, user.ID)

	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()

	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (pgStore *PostgresUserStore) GetUserByToken(scope, plaintextPassword string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(plaintextPassword))

	query := `
  SELECT u.id, u.username, u.email, u.password_hash, u.bio, u.created_at, u.updated_at
  FROM users u
  INNER JOIN tokens t ON t.user_id = u.id
  WHERE t.hash = $1 AND t.scope = $2 and t.expiry > $3`

	user := &User{
		PasswordHash: password{},
	}

	err := pgStore.db.QueryRow(query, tokenHash[:], scope, time.Now()).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash.hash,
		&user.Bio,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}
