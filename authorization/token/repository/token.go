package repository

import (
	"context"
	"fmt"
	"time"

	permission "github.com/fabric8-services/fabric8-auth/authorization/permission/repository"
	"github.com/fabric8-services/fabric8-auth/errors"
	"github.com/fabric8-services/fabric8-auth/gormsupport"
	"github.com/fabric8-services/fabric8-auth/log"

	"github.com/goadesign/goa"
	"github.com/jinzhu/gorm"
	errs "github.com/pkg/errors"
	"github.com/satori/go.uuid"
)

// Token represents a single instance of an oauth token
type Token struct {
	gormsupport.Lifecycle

	// This is the primary key value
	TokenID uuid.UUID `sql:"type:uuid default uuid_generate_v4()" gorm:"primary_key;column:token_id"`

	IdentityID uuid.UUID

	Status int

	TokenType string

	// The timestamp when the token will expire
	ExpiryTime time.Time
}

// TableName overrides the table name settings in Gorm to force a specific table name
// in the database.
func (m Token) TableName() string {
	return "token"
}

func (m *Token) Valid() bool {
	return m.Status == 0
}

func (m *Token) HasStatus(status int) bool {
	return m.Status&status == status
}

func (m *Token) SetStatus(status int, value bool) {
	if value {
		m.Status |= status
	} else {
		m.Status &^= status
	}
}

// TokenPrivilege is a simple many-to-many table used to link privileges to tokens
type TokenPrivilege struct {
	TokenID          uuid.UUID `sql:"type:uuid default uuid_generate_v4()" gorm:"primary_key;column:token_id"`
	PrivilegeCacheID uuid.UUID `sql:"type:uuid default uuid_generate_v4()" gorm:"primary_key;column:privilege_cache_id"`
}

func (m TokenPrivilege) TableName() string {
	return "token_privilege"
}

// GormTokenRepository is the implementation of the storage interface for Token.
type GormTokenRepository struct {
	db *gorm.DB
}

// NewTokenRepository creates a new storage type.
func NewTokenRepository(db *gorm.DB) TokenRepository {
	return &GormTokenRepository{db: db}
}

func (m *GormTokenRepository) TableName() string {
	return "token"
}

func (m *GormTokenRepository) TokenPrivilegeTableName() string {
	return "token_privilege"
}

// TokenRepository represents the storage interface.
type TokenRepository interface {
	CheckExists(ctx context.Context, id uuid.UUID) (bool, error)
	Load(ctx context.Context, id uuid.UUID) (*Token, error)
	Create(ctx context.Context, token *Token) error
	Save(ctx context.Context, token *Token) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListForIdentity(ctx context.Context, id uuid.UUID) ([]Token, error)
	CreatePrivilege(ctx context.Context, privilege *TokenPrivilege) error
	ListPrivileges(ctx context.Context, tokenID uuid.UUID) ([]permission.PrivilegeCache, error)
	SetStatusFlagsForIdentity(ctx context.Context, identityID uuid.UUID, status int) error
	CleanupExpiredTokens(ctx context.Context, retentionHours int) error
}

// CRUD Functions

// CheckExists returns true if the given ID exists otherwise returns an error
func (m *GormTokenRepository) CheckExists(ctx context.Context, id uuid.UUID) (bool, error) {
	defer goa.MeasureSince([]string{"goa", "db", "token", "exists"}, time.Now())

	var exists bool
	query := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %[1]s
			WHERE
				token_id=$1
		)`, m.TableName())

	err := m.db.CommonDB().QueryRow(query, id).Scan(&exists)
	if err == nil && !exists {
		return exists, errors.NewNotFoundError(m.TableName(), id.String())
	}
	if err != nil {
		return false, errors.NewInternalError(ctx, errs.Wrapf(err, "unable to verify if %s exists", m.TableName()))
	}
	return exists, nil
}

// Load returns a single Token as a Database Model
func (m *GormTokenRepository) Load(ctx context.Context, id uuid.UUID) (*Token, error) {
	defer goa.MeasureSince([]string{"goa", "db", "_token", "load"}, time.Now())

	var native Token
	err := m.db.Table(m.TableName()).Where("token_id = ?", id).Find(&native).Error
	if err == gorm.ErrRecordNotFound {
		return nil, errs.WithStack(errors.NewNotFoundError("token", id.String()))
	}

	return &native, errs.WithStack(err)
}

// Create creates a new record.
func (m *GormTokenRepository) Create(ctx context.Context, token *Token) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "create"}, time.Now())

	// If no identifier has been specified for the new token, then generate one
	if token.TokenID == uuid.Nil {
		token.TokenID = uuid.NewV4()
	}

	err := m.db.Create(token).Error
	if err != nil {
		if gormsupport.IsUniqueViolation(err, "token_pkey") {
			return errors.NewDataConflictError(fmt.Sprintf("token with ID %s already exists", token.TokenID))
		}

		log.Error(ctx, map[string]interface{}{
			"token_id": token.TokenID,
			"err":      err,
		}, "unable to create the token")
		return errs.WithStack(err)
	}

	log.Info(ctx, map[string]interface{}{
		"token_id": token.TokenID,
	}, "Token created!")
	return nil
}

// Save modifies a single record.
func (m *GormTokenRepository) Save(ctx context.Context, token *Token) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "save"}, time.Now())

	obj, err := m.Load(ctx, token.TokenID)
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"token_id": token.TokenID,
			"err":      err,
		}, "unable to update token")
		return errs.WithStack(err)
	}

	err = m.db.Model(obj).Updates(token).Error
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"token_id": token.TokenID,
			"err":      err,
		}, "unable to update the token")

		return errs.WithStack(err)
	}

	log.Debug(ctx, map[string]interface{}{
		"token_id": token.TokenID,
	}, "Token saved!")

	return nil
}

// Delete removes a single record.
func (m *GormTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "delete"}, time.Now())

	obj := Token{TokenID: id}
	result := m.db.Delete(obj)

	if result.Error != nil {
		log.Error(ctx, map[string]interface{}{
			"token_id": id,
			"err":      result.Error,
		}, "unable to delete the token")
		return errs.WithStack(result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.NewNotFoundError("token", id.String())
	}

	log.Debug(ctx, map[string]interface{}{
		"token_id": id,
	}, "Token deleted!")

	return nil
}

func (m *GormTokenRepository) ListForIdentity(ctx context.Context, identityID uuid.UUID) ([]Token, error) {
	defer goa.MeasureSince([]string{"goa", "db", "token", "ListForIdentity"}, time.Now())
	var rows []Token

	err := m.db.Model(&Token{}).Where("identity_id = ?", identityID).Find(&rows).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, errs.WithStack(err)
	}
	return rows, nil
}

func (m *GormTokenRepository) CreatePrivilege(ctx context.Context, privilege *TokenPrivilege) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "CreatePrivilege"}, time.Now())

	err := m.db.Create(privilege).Error
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"token_id":           privilege.TokenID,
			"privilege_cache_id": privilege.PrivilegeCacheID,
			"err":                err,
		}, "unable to create the token privilege")
		return errs.WithStack(err)
	}

	log.Info(ctx, map[string]interface{}{
		"token_id":           privilege.TokenID,
		"privilege_cache_id": privilege.PrivilegeCacheID,
	}, "Token privilege created!")
	return nil
}

func (m *GormTokenRepository) ListPrivileges(ctx context.Context, tokenID uuid.UUID) ([]permission.PrivilegeCache, error) {
	defer goa.MeasureSince([]string{"goa", "db", "token", "ListPrivileges"}, time.Now())
	var rows []permission.PrivilegeCache

	err := m.db.Table("privilege_cache").Where(fmt.Sprintf(`
        deleted_at IS NULL AND privilege_cache_id IN (SELECT tp.privilege_cache_id FROM %s tp WHERE tp.token_id = ?)
        `, m.TokenPrivilegeTableName()),
		tokenID).Scan(&rows).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, errs.WithStack(err)
	}
	return rows, nil
}

func (m *GormTokenRepository) SetStatusFlagsForIdentity(ctx context.Context, identityID uuid.UUID, status int) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "SetStatusFlagsForIdentity"}, time.Now())

	err := m.db.Exec("UPDATE token SET status = status | ? WHERE identity_id = ? AND deleted_at IS NULL", status, identityID).Error
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"identity_id": identityID.String(),
			"status":      status,
			"err":         err,
		}, "unable to update token status")
		return errs.WithStack(err)
	}

	log.Info(ctx, map[string]interface{}{
		"identity_id": identityID.String(),
		"status":      status,
	}, "Token status values updated")
	return nil
}

func (m *GormTokenRepository) CleanupExpiredTokens(ctx context.Context, retentionHours int) error {
	defer goa.MeasureSince([]string{"goa", "db", "token", "CleanupExpiredTokens"}, time.Now())

	threshold := time.Now().Add(time.Duration(-retentionHours) * time.Hour)
	err := m.db.Exec("DELETE FROM token WHERE expiry_time < ?", threshold).Error
	if err != nil {
		log.Error(ctx, map[string]interface{}{
			"err": err,
		}, "unable to cleanup expired tokens")
		return errs.WithStack(err)
	}

	log.Info(ctx, map[string]interface{}{}, "Expired tokens cleaned up")
	return nil
}
