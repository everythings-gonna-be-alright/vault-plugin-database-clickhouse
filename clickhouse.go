package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-secure-stdlib/strutil"
	"github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/database/helper/connutil"
	"github.com/hashicorp/vault/sdk/database/helper/dbutil"
	"github.com/hashicorp/vault/sdk/helper/dbtxn"
	"github.com/hashicorp/vault/sdk/helper/template"
	"strings"
)

const (
	clickhouseTypeName             = "clickhouse"
	defaultChangePasswordStatement = "ALTER USER '{{username}}' IDENTIFIED WITH plaintext_password '{{password}}' ON CLUSTER '{{cluster}}';"
	expirationFormat               = "2006-01-02 15:04:05"
	defaultUserNameTemplate        = `{{ printf "v-%s-%s-%s-%s" (.DisplayName | truncate 8) (.RoleName | truncate 8) (random 20) (unix_time) | truncate 63 }}`
)

func New() (interface{}, error) {
	db := new()
	// Wrap the plugin with middleware to sanitize errors
	dbType := dbplugin.NewDatabaseErrorSanitizerMiddleware(db, db.secretValues)
	return dbType, nil
}

func new() *Clickhouse {
	connProducer := &connutil.SQLConnectionProducer{}
	connProducer.Type = clickhouseTypeName

	db := &Clickhouse{
		SQLConnectionProducer: connProducer,
	}

	return db
}

type Clickhouse struct {
	*connutil.SQLConnectionProducer

	usernameProducer template.StringTemplate
}

func (p *Clickhouse) Initialize(ctx context.Context, req dbplugin.InitializeRequest) (dbplugin.InitializeResponse, error) {
	newConf, err := p.SQLConnectionProducer.Init(ctx, req.Config, req.VerifyConnection)
	if err != nil {
		return dbplugin.InitializeResponse{}, err
	}

	usernameTemplate, err := strutil.GetString(req.Config, "username_template")
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("failed to retrieve username_template: %w", err)
	}
	if usernameTemplate == "" {
		usernameTemplate = defaultUserNameTemplate
	}

	up, err := template.NewTemplate(template.Template(usernameTemplate))
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("unable to initialize username template: %w", err)
	}
	p.usernameProducer = up

	_, err = p.usernameProducer.Generate(dbplugin.UsernameMetadata{})
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("invalid username template: %w", err)
	}

	resp := dbplugin.InitializeResponse{
		Config: newConf,
	}
	return resp, nil
}

func (p *Clickhouse) Type() (string, error) {
	return clickhouseTypeName, nil
}

func (p *Clickhouse) getConnection(ctx context.Context) (*sql.DB, error) {
	db, err := p.Connection(ctx)
	if err != nil {
		return nil, err
	}

	return db.(*sql.DB), nil
}

func (p *Clickhouse) UpdateUser(ctx context.Context, req dbplugin.UpdateUserRequest) (dbplugin.UpdateUserResponse, error) {
	if req.Username == "" {
		return dbplugin.UpdateUserResponse{}, fmt.Errorf("missing username")
	}
	if req.Password == nil && req.Expiration == nil {
		return dbplugin.UpdateUserResponse{}, fmt.Errorf("no changes requested")
	}

	merr := &multierror.Error{}
	if req.Password != nil {
		err := p.changeUserPassword(ctx, req.Username, req.Password)
		merr = multierror.Append(merr, err)
	}

	//if req.Expiration != nil {
	//	err := p.changeUserExpiration(ctx, req.Username, req.Expiration)
	//	merr = multierror.Append(merr, err)
	//}
	return dbplugin.UpdateUserResponse{}, merr.ErrorOrNil()
}

func (p *Clickhouse) changeUserPassword(ctx context.Context, username string, changePass *dbplugin.ChangePassword) error {
	stmts := changePass.Statements.Commands
	if len(stmts) == 0 {
		stmts = []string{defaultChangePasswordStatement}
	}

	cluster, err := p.getCluster(ctx)
	if err != nil {
		return err
	}

	password := changePass.NewPassword
	if password == "" {
		return fmt.Errorf("missing password")
	}

	p.Lock()
	defer p.Unlock()

	db, err := p.getConnection(ctx)
	if err != nil {
		return fmt.Errorf("unable to get connection: %w", err)
	}

	// Check if the role exists
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT c > 0 AS exists FROM ( SELECT count() AS c FROM system.users WHERE name=$1 );", username).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("user does not appear to exist: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("unable to start transaction: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range stmts {
		for _, query := range strutil.ParseArbitraryStringSlice(stmt, ";") {
			query = strings.TrimSpace(query)
			if len(query) == 0 {
				continue
			}

			m := map[string]string{
				"name":     username,
				"username": username,
				"password": password,
				"cluster":  cluster,
			}
			if err := dbtxn.ExecuteTxQueryDirect(ctx, tx, m, query); err != nil {
				return fmt.Errorf("failed to execute query: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (p *Clickhouse) NewUser(ctx context.Context, req dbplugin.NewUserRequest) (dbplugin.NewUserResponse, error) {
	if len(req.Statements.Commands) == 0 {
		return dbplugin.NewUserResponse{}, dbutil.ErrEmptyCreationStatement
	}

	p.Lock()
	defer p.Unlock()

	cluster, err := p.getCluster(ctx)
	if err != nil {
		return dbplugin.NewUserResponse{}, err
	}

	username, err := p.usernameProducer.Generate(req.UsernameConfig)
	if err != nil {
		return dbplugin.NewUserResponse{}, err
	}

	expirationStr := req.Expiration.Format(expirationFormat)

	db, err := p.getConnection(ctx)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("unable to get connection: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("unable to start transaction: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range req.Statements.Commands {
		for _, query := range strutil.ParseArbitraryStringSlice(stmt, ";") {
			query = strings.TrimSpace(query)
			if len(query) == 0 {
				continue
			}

			m := map[string]string{
				"name":       username,
				"username":   username,
				"password":   req.Password,
				"expiration": expirationStr,
				"cluster":    cluster,
			}
			if err := dbtxn.ExecuteTxQueryDirect(ctx, tx, m, query); err != nil {
				return dbplugin.NewUserResponse{}, fmt.Errorf("failed to execute query: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return dbplugin.NewUserResponse{}, err
	}

	resp := dbplugin.NewUserResponse{
		Username: username,
	}
	return resp, nil
}

func (p *Clickhouse) DeleteUser(ctx context.Context, req dbplugin.DeleteUserRequest) (dbplugin.DeleteUserResponse, error) {
	p.Lock()
	defer p.Unlock()

	if len(req.Statements.Commands) == 0 {
		return dbplugin.DeleteUserResponse{}, p.defaultDeleteUser(ctx, req.Username)
	}

	return dbplugin.DeleteUserResponse{}, p.customDeleteUser(ctx, req.Username, req.Statements.Commands)
}

func (p *Clickhouse) customDeleteUser(ctx context.Context, username string, revocationStmts []string) error {
	db, err := p.getConnection(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		tx.Rollback()
	}()

	for _, stmt := range revocationStmts {
		for _, query := range strutil.ParseArbitraryStringSlice(stmt, ";") {
			query = strings.TrimSpace(query)
			if len(query) == 0 {
				continue
			}

			m := map[string]string{
				"name":     username,
				"username": username,
			}
			if err := dbtxn.ExecuteTxQueryDirect(ctx, tx, m, query); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (p *Clickhouse) defaultDeleteUser(ctx context.Context, username string) error {
	db, err := p.getConnection(ctx)
	if err != nil {
		return err
	}

	cluster, err := p.getCluster(ctx)
	if err != nil {
		return err
	}

	// Check if the user exists
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT c > 0 AS exists FROM ( SELECT count() AS c FROM system.users WHERE name=$1 );", username).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if !exists {
		return nil
	}

	// Drop this user
	_, err = db.ExecContext(ctx, "DROP USER IF EXISTS $1 ON CLUSTER $2;", username, cluster)
	if err != nil {
		return err
	}

	defer db.Close()

	return nil
}

func (p *Clickhouse) secretValues() map[string]string {
	return map[string]string{
		p.Password: "[password]",
	}
}

// Fetch cluster name from system.setting for on_cluster queries
func (p *Clickhouse) getCluster(ctx context.Context) (string, error) {
	db, err := p.getConnection(ctx)
	if err != nil {
		return "", err
	}

	// Check if the user exists
	var cluster string
	err = db.QueryRowContext(ctx, "SELECT DISTINCT cluster FROM system.clusters;").Scan(&cluster)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	return cluster, nil
}
