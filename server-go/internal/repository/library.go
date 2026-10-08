package repository

import (
	"database/sql"
	"fmt"
)

type LibraryRepository struct {
	db *sql.DB
}

func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

// GetAllLibraryIDs mirrors Library.getAllLibraryIds (server/models/Library.js):
// every library id, ordered by displayOrder ascending.
func (r *LibraryRepository) GetAllLibraryIDs() ([]string, error) {
	rows, err := r.db.Query(`SELECT id FROM libraries ORDER BY displayOrder ASC`)
	if err != nil {
		return nil, fmt.Errorf("querying library ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning library id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading library ids: %w", err)
	}
	return ids, nil
}
