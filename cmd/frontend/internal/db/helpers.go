package db

import (
	"context"
	"database/sql"
	"log"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// Transaction calls f within a transaction, rolling back if any error is
// returned by the function.
func Transaction(ctx context.Context, db *sql.DB, f func(tx *sql.Tx) error) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Transaction")
	defer func() {
		if err != nil {
			ext.Error.Set(span, true)
			span.SetTag("err", err.Error())
		}
		span.Finish()
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err2 := tx.Rollback(); err2 != nil {
				log.Println("Transaction Rollback failed:", err2)
			}
		}
	}()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}
