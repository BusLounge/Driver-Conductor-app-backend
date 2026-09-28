package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReportIssue represents a row in the report_issues table
type ReportIssue struct {
	ID                 string         `json:"id"`
	ScheduledTripID    sql.NullString `json:"scheduled_trip_id"`
	ActiveTripID       sql.NullString `json:"active_trip_id"`
	ReportedByID       string         `json:"reported_by_id"`
	IssueType          string         `json:"issue_type"`
	Priority           string         `json:"priority"`
	Status             string         `json:"status"`
	Description        string         `json:"description"`
	Latitude           sql.NullFloat64 `json:"latitude"`
	Longitude          sql.NullFloat64 `json:"longitude"`
	LocationAddress    sql.NullString `json:"location_address"`
	ImageURL           sql.NullString `json:"image_url"`
	ResolvedAt         sql.NullTime   `json:"resolved_at"`
	ResolvedByID       sql.NullString `json:"resolved_by_id"`
	ResolutionNotes    sql.NullString `json:"resolution_notes"`
	NotifiedPassengers sql.NullBool   `json:"notified_passengers"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	// Joined field (not a column in report_issues itself)
	ReportedByName string `json:"reported_by_name"`
}

// ReportIssuesRepository handles database operations for the report_issues table
type ReportIssuesRepository struct {
	db DB
}

// NewReportIssuesRepository creates a new ReportIssuesRepository
func NewReportIssuesRepository(db DB) *ReportIssuesRepository {
	return &ReportIssuesRepository{db: db}
}

// Create inserts a new report issue and returns the created row
func (r *ReportIssuesRepository) Create(report *ReportIssue) error {
	if report.ID == "" {
		report.ID = uuid.New().String()
	}

	query := `
		INSERT INTO report_issues (
			id, scheduled_trip_id, active_trip_id, reported_by_id,
			issue_type, priority, status, description,
			latitude, longitude, location_address, image_url
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12
		)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(
		query,
		report.ID, report.ScheduledTripID, report.ActiveTripID, report.ReportedByID,
		report.IssueType, report.Priority, report.Status, report.Description,
		report.Latitude, report.Longitude, report.LocationAddress, report.ImageURL,
	).Scan(&report.CreatedAt, &report.UpdatedAt)

	return err
}

// GetByID retrieves a report issue by its ID, joining users to get reporter name
func (r *ReportIssuesRepository) GetByID(reportID string) (*ReportIssue, error) {
	query := `
		SELECT
			ri.id, ri.scheduled_trip_id, ri.active_trip_id, ri.reported_by_id,
			ri.issue_type, ri.priority, ri.status, ri.description,
			ri.latitude, ri.longitude, ri.location_address, ri.image_url,
			ri.resolved_at, ri.resolved_by_id, ri.resolution_notes,
			ri.notified_passengers, ri.created_at, ri.updated_at,
			COALESCE(
				NULLIF(TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')), ''),
				NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''),
				''
			) AS reported_by_name
		FROM report_issues ri
		LEFT JOIN users u ON u.id = ri.reported_by_id
		LEFT JOIN bus_staff bs ON bs.user_id = ri.reported_by_id
		WHERE ri.id = $1
	`

	report := &ReportIssue{}
	err := r.db.QueryRow(query, reportID).Scan(
		&report.ID, &report.ScheduledTripID, &report.ActiveTripID, &report.ReportedByID,
		&report.IssueType, &report.Priority, &report.Status, &report.Description,
		&report.Latitude, &report.Longitude, &report.LocationAddress, &report.ImageURL,
		&report.ResolvedAt, &report.ResolvedByID, &report.ResolutionNotes,
		&report.NotifiedPassengers, &report.CreatedAt, &report.UpdatedAt,
		&report.ReportedByName,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("report not found")
		}
		return nil, err
	}

	return report, nil
}

// GetByReporterID retrieves all reports submitted by a given user, newest first
func (r *ReportIssuesRepository) GetByReporterID(userID string) ([]ReportIssue, error) {
	query := `
		SELECT
			ri.id, ri.scheduled_trip_id, ri.active_trip_id, ri.reported_by_id,
			ri.issue_type, ri.priority, ri.status, ri.description,
			ri.latitude, ri.longitude, ri.location_address, ri.image_url,
			ri.resolved_at, ri.resolved_by_id, ri.resolution_notes,
			ri.notified_passengers, ri.created_at, ri.updated_at,
			COALESCE(
				NULLIF(TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')), ''),
				NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''),
				''
			) AS reported_by_name
		FROM report_issues ri
		LEFT JOIN users u ON u.id = ri.reported_by_id
		LEFT JOIN bus_staff bs ON bs.user_id = ri.reported_by_id
		WHERE ri.reported_by_id = $1
		ORDER BY ri.created_at DESC
	`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// GetByTripID retrieves all reports associated with a scheduled or active trip
func (r *ReportIssuesRepository) GetByTripID(tripID string) ([]ReportIssue, error) {
	query := `
		SELECT
			ri.id, ri.scheduled_trip_id, ri.active_trip_id, ri.reported_by_id,
			ri.issue_type, ri.priority, ri.status, ri.description,
			ri.latitude, ri.longitude, ri.location_address, ri.image_url,
			ri.resolved_at, ri.resolved_by_id, ri.resolution_notes,
			ri.notified_passengers, ri.created_at, ri.updated_at,
			COALESCE(
				NULLIF(TRIM(COALESCE(bs.first_name, '') || ' ' || COALESCE(bs.last_name, '')), ''),
				NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''),
				''
			) AS reported_by_name
		FROM report_issues ri
		LEFT JOIN users u ON u.id = ri.reported_by_id
		LEFT JOIN bus_staff bs ON bs.user_id = ri.reported_by_id
		WHERE ri.scheduled_trip_id = $1 OR ri.active_trip_id = $1
		ORDER BY ri.created_at DESC
	`

	rows, err := r.db.Query(query, tripID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// scanRows scans multiple result rows into a slice of ReportIssue
func (r *ReportIssuesRepository) scanRows(rows *sql.Rows) ([]ReportIssue, error) {
	var reports []ReportIssue
	for rows.Next() {
		var report ReportIssue
		err := rows.Scan(
			&report.ID, &report.ScheduledTripID, &report.ActiveTripID, &report.ReportedByID,
			&report.IssueType, &report.Priority, &report.Status, &report.Description,
			&report.Latitude, &report.Longitude, &report.LocationAddress, &report.ImageURL,
			&report.ResolvedAt, &report.ResolvedByID, &report.ResolutionNotes,
			&report.NotifiedPassengers, &report.CreatedAt, &report.UpdatedAt,
			&report.ReportedByName,
		)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

// ToJSON converts a ReportIssue to a JSON-friendly map matching what the Flutter frontend expects
func (report *ReportIssue) ToJSON() map[string]interface{} {
	result := map[string]interface{}{
		"id":               report.ID,
		"reported_by_id":   report.ReportedByID,
		"reported_by_name": report.ReportedByName,
		"issue_type":       report.IssueType,
		"priority":         report.Priority,
		"status":           report.Status,
		"description":      report.Description,
		"created_at":       report.CreatedAt.Format(time.RFC3339),
		"updated_at":       report.UpdatedAt.Format(time.RFC3339),
	}

	if report.ScheduledTripID.Valid {
		result["scheduled_trip_id"] = report.ScheduledTripID.String
	}
	if report.ActiveTripID.Valid {
		result["active_trip_id"] = report.ActiveTripID.String
	}
	if report.LocationAddress.Valid {
		result["location_address"] = report.LocationAddress.String
	}
	if report.ImageURL.Valid {
		result["image_url"] = report.ImageURL.String
	}
	if report.ResolvedAt.Valid {
		result["resolved_at"] = report.ResolvedAt.Time.Format(time.RFC3339)
	}
	if report.ResolvedByID.Valid {
		result["resolved_by_id"] = report.ResolvedByID.String
	}
	if report.ResolutionNotes.Valid {
		result["resolution_notes"] = report.ResolutionNotes.String
	}

	return result
}
