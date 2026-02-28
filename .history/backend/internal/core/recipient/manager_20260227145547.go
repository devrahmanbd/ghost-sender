func (r *RecipientRepository) Create(ctx context.Context, rec *Recipient) error {
	fmt.Printf("🟢 DEBUG Repo.Create: list_id=%s email=%s\n", rec.ListID, rec.Email)

	query := `
		INSERT INTO recipients (list_id, email, name, first_name, last_name, status, custom_fields, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at, updated_at`

	customFieldsJSON, _ := json.Marshal(rec.Attributes)
	metaJSON, _ := json.Marshal(rec.Metadata)

	err := r.db.QueryRowContext(ctx, query,
		rec.ListID,        // $1
		rec.Email,         // $2
		rec.Name,          // $3
		rec.FirstName,     // $4
		rec.LastName,      // $5
		rec.Status,        // $6
		customFieldsJSON,  // $7
		metaJSON,          // $8
		time.Now(),        // $9
		time.Now(),        // $10
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		fmt.Printf("🔴 DEBUG DB Scan ERROR: %v\n", err)
		return err
	}

	fmt.Printf("🟢 DEBUG Repo.Create SUCCESS id=%s\n", rec.ID)
	return nil
}
