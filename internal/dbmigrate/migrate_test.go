package dbmigrate

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedAppAndProjectStreamsAreIndependent(t *testing.T) {
	appDSN := "file:" + filepath.Join(t.TempDir(), "app.sqlite")
	appRunner, err := OpenApp(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := appRunner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := appRunner.Close(); err != nil {
		t.Fatal(err)
	}
	projectDSN := "file:" + filepath.Join(t.TempDir(), "project.sqlite")
	projectRunner, err := OpenProject(projectDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := projectRunner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := projectRunner.Close(); err != nil {
		t.Fatal(err)
	}

	appTables := tableNames(t, appDSN)
	projectTables := tableNames(t, projectDSN)
	if !containsTable(appTables, "recent_projects") || containsTable(appTables, "projects") || containsTable(appTables, "actors") {
		t.Fatalf("app tables = %v", appTables)
	}
	if !containsTable(projectTables, "projects") || !containsTable(projectTables, "actors") || containsTable(projectTables, "recent_projects") {
		t.Fatalf("project tables = %v", projectTables)
	}
	for _, table := range []string{"upload_stashed", "file_objects", "files", "integrity_scans", "asset_maintenance_runs"} {
		if !containsTable(projectTables, table) {
			t.Fatalf("project tables missing %s: %v", table, projectTables)
		}
	}
}

func TestProjectCreationReferenceAppMigrationPreservesSessionsAndDowngradesWaitingState(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-creation-references-app.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(2); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-28T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO project_creation_sessions(id,uuid,idempotency_key,input_text,status,planned_project_uuid,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000001','migration-reference-session','Create with references','creating_project','019c0000-0000-7000-8000-000000000002',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "project_creation_session_references") {
		t.Fatal("reference app migration did not create its manifest table")
	}
	if _, err := db.Exec(`UPDATE project_creation_sessions SET status='awaiting_references' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_creation_session_references(uuid,project_creation_session_id,position,upload_uuid,file_uuid,original_filename,declared_mime_type,declared_byte_size,status,created_at,updated_at) VALUES('019c0000-0000-7000-8000-000000000003',1,1,'019c0000-0000-7000-8000-000000000004','019c0000-0000-7000-8000-000000000005','reference.png','image/png',128,'pending',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(2); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "project_creation_session_references") {
		t.Fatal("reference app down migration retained its manifest table")
	}
	var status, input string
	if err := db.QueryRow(`SELECT status,input_text FROM project_creation_sessions WHERE id=1`).Scan(&status, &input); err != nil {
		t.Fatal(err)
	}
	if status != "creating_project" || input != "Create with references" {
		t.Fatalf("downgraded session status=%q input=%q", status, input)
	}
}

func TestProjectCreationReferenceFileMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-creation-reference-files.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "project_creation_reference_files") {
		t.Fatal("project reference migration did not create its binding table")
	}
	if err := runner.Down(4); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "project_creation_reference_files") {
		t.Fatal("project reference down migration retained its binding table")
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "project_creation_reference_files") {
		t.Fatal("project reference migration did not recreate its binding table")
	}
}

func TestProjectCreationReferencePlanAppMigrationPreservesLegacyRowsAndChecksPlans(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-creation-reference-plans-app.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenApp(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-31T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO project_creation_sessions(id,uuid,idempotency_key,input_text,status,planned_project_uuid,created_at,updated_at) VALUES(1,'019c1000-0000-7000-8000-000000000001','reference-plan-app','Create from image','awaiting_references','019c1000-0000-7000-8000-000000000002',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_creation_session_references(id,uuid,project_creation_session_id,position,upload_uuid,file_uuid,original_filename,declared_mime_type,declared_byte_size,status,created_at,updated_at) VALUES(1,'019c1000-0000-7000-8000-000000000003',1,1,'019c1000-0000-7000-8000-000000000004','019c1000-0000-7000-8000-000000000005','fox.png','image/png',128,'ready',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var role, title, instruction, source string
	var included int
	if err := db.QueryRow(`SELECT reference_role,title,instruction,include_in_yolo,plan_source FROM project_creation_session_references WHERE id=1`).Scan(&role, &title, &instruction, &included, &source); err != nil {
		t.Fatal(err)
	}
	if role != "auto" || title != "" || instruction != "" || included != 1 || source != "system_default" {
		t.Fatalf("migrated app plan=%q/%q/%q/%d/%q", role, title, instruction, included, source)
	}
	if _, err := db.Exec(`UPDATE project_creation_session_references SET reference_role='portrait' WHERE id=1`); err == nil {
		t.Fatal("app reference plan accepted an invalid role")
	}
	if err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "project_creation_session_references", "reference_role") {
		t.Fatal("app reference-plan columns survived rollback")
	}
	var filename string
	if err := db.QueryRow(`SELECT original_filename FROM project_creation_session_references WHERE id=1`).Scan(&filename); err != nil || filename != "fox.png" {
		t.Fatalf("rolled-back app reference filename=%q err=%v", filename, err)
	}
}

func TestProjectCreationReferencePlanProjectMigrationPreservesLegacyRowsAndChecksPlans(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-creation-reference-plans-project.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-31T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'019c1000-0000-7000-8000-000000000101','Reference plan',1,33,'` + now + `','` + now + `')`,
		`INSERT INTO file_objects(id,uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at) VALUES(1,'019c1000-0000-7000-8000-000000000102',1,'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','objects/reference.png','image/png','png',128,16,16,'ready','` + now + `','` + now + `')`,
		`INSERT INTO files(id,uuid,project_id,file_object_id,kind,purpose,original_filename,display_name,source_type,metadata_json,created_at) VALUES(1,'019c1000-0000-7000-8000-000000000103',1,1,'image','project_chatbot_reference','fox.png','Fox','imported','{}','` + now + `')`,
		`INSERT INTO project_creation_reference_files(id,uuid,project_id,creation_session_uuid,reference_uuid,position,file_id,created_at) VALUES(1,'019c1000-0000-7000-8000-000000000104',1,'019c1000-0000-7000-8000-000000000105','019c1000-0000-7000-8000-000000000106',1,1,'` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var role, title, source, updatedAt string
	var included int
	if err := db.QueryRow(`SELECT reference_role,title,include_in_yolo,plan_source,updated_at FROM project_creation_reference_files WHERE id=1`).Scan(&role, &title, &included, &source, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if role != "auto" || title != "" || included != 1 || source != "system_default" || updatedAt == "" {
		t.Fatalf("migrated project plan=%q/%q/%d/%q updated=%q", role, title, included, source, updatedAt)
	}
	if _, err := db.Exec(`UPDATE project_creation_reference_files SET include_in_yolo=2 WHERE id=1`); err == nil {
		t.Fatal("project reference plan accepted a non-boolean include flag")
	}
	if err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "project_creation_reference_files", "reference_role") {
		t.Fatal("project reference-plan columns survived rollback")
	}
	var position int
	if err := db.QueryRow(`SELECT position FROM project_creation_reference_files WHERE id=1`).Scan(&position); err != nil || position != 1 {
		t.Fatalf("rolled-back project reference position=%d err=%v", position, err)
	}
}

func TestComicSectionPageRoleMigrationBackfillsAndConstrainsSpecialPages(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-section-page-roles.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(3); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-29T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000201','Page roles',1,31,'` + now + `','` + now + `')`,
		`INSERT INTO actors(id,uuid,name,kind,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000202','Owner','local_user','` + now + `','` + now + `')`,
		`INSERT INTO chapters(id,uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000203',1,1,1,'vol01.ch01',1,'Book','` + now + `','` + now + `')`,
		`INSERT INTO chapter_comic_states(id,uuid,chapter_id,status,revision,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000204',1,'draft',0,'` + now + `','` + now + `')`,
		`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,title,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000205',1,1,1,'Body','` + now + `','` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed pre-role comic schema: %v\n%s", err, statement)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var pageRole string
	if err := db.QueryRow(`SELECT page_role FROM comic_sections WHERE id=1`).Scan(&pageRole); err != nil || pageRole != "body" {
		t.Fatalf("backfilled page_role=%q error=%v", pageRole, err)
	}
	if _, err := db.Exec(`UPDATE comic_sections SET page_role='invalid' WHERE id=1`); err == nil {
		t.Fatal("page_role CHECK accepted an invalid role")
	}
	if _, err := db.Exec(`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,page_role,title,created_at,updated_at) VALUES(2,'019c0000-0000-7000-8000-000000000206',1,1,2,'front_cover','Front','` + now + `','` + now + `')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,page_role,title,created_at,updated_at) VALUES(3,'019c0000-0000-7000-8000-000000000207',1,1,3,'front_cover','Duplicate','` + now + `','` + now + `')`); err == nil {
		t.Fatal("active special-role unique index accepted a second front cover")
	}
	if _, err := db.Exec(`UPDATE comic_sections SET deleted_at='` + now + `' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,page_role,title,created_at,updated_at) VALUES(3,'019c0000-0000-7000-8000-000000000207',1,1,3,'front_cover','Replacement','` + now + `','` + now + `')`); err != nil {
		t.Fatalf("soft-deleted front cover did not release role: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM comic_sections WHERE page_role <> 'body'`); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(3); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "comic_sections", "page_role") {
		t.Fatal("page-role down migration retained page_role")
	}
}

func TestComicSectionPremiseAssetSelectionMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-section-premise-selections.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "comic_section_premise_asset_selections") {
		t.Fatal("selection migration did not create its relation table")
	}
	var sectionFK, assetFK int
	rows, err := db.Query(`SELECT "table" FROM pragma_foreign_key_list('comic_section_premise_asset_selections')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		if table == "comic_sections" {
			sectionFK++
		}
		if table == "premise_assets" {
			assetFK++
		}
	}
	_ = rows.Close()
	if sectionFK != 1 || assetFK != 1 {
		t.Fatalf("selection foreign keys section=%d asset=%d", sectionFK, assetFK)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(2); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "comic_section_premise_asset_selections") {
		t.Fatal("selection down migration retained its relation table")
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestComicSectionPageRoleMigrationRejectsLossyDown(t *testing.T) {
	tests := []struct {
		name, pageRole, snapshotRole, durableKind string
		deleted                                   bool
	}{
		{name: "active front cover", pageRole: "front_cover"},
		{name: "soft-deleted back cover", pageRole: "back_cover", deleted: true},
		{name: "cover in durable snapshot", pageRole: "body", snapshotRole: "front_cover"},
		{name: "durable export v6", pageRole: "body", durableKind: "export_record_v6"},
		{name: "queued export with v6 parameters", pageRole: "body", durableKind: "export_task_v6"},
		{name: "body image task v5", pageRole: "body", durableKind: "image_task_v5"},
		{name: "vertical yolo workflow v5", pageRole: "body", durableKind: "yolo_workflow_v5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := "file:" + filepath.Join(t.TempDir(), "lossy-page-role-down.sqlite") + "?_pragma=foreign_keys(1)"
			runner, err := OpenProject(dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runner.Close() })
			if err := runner.Up(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			const now = "2026-08-29T00:00:00Z"
			statements := []string{
				`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000301','Lossy page role',1,32,'` + now + `','` + now + `')`,
				`INSERT INTO actors(id,uuid,name,kind,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000302','Owner','local_user','` + now + `','` + now + `')`,
				`INSERT INTO chapters(id,uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000303',1,1,1,'vol01.ch01',1,'Book','` + now + `','` + now + `')`,
				`INSERT INTO chapter_comic_states(id,uuid,chapter_id,status,revision,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000304',1,'draft',0,'` + now + `','` + now + `')`,
			}
			for _, statement := range statements {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			var deletedAt any
			if test.deleted {
				deletedAt = now
			}
			if _, err := db.Exec(`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,page_role,title,deleted_at,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000305',1,1,1,?,'Page',?,?,?)`, test.pageRole, deletedAt, now, now); err != nil {
				t.Fatal(err)
			}
			if test.snapshotRole != "" {
				snapshotJSON := `{"version":3,"sections":[{"uuid":"019c0000-0000-7000-8000-000000000305","page_role":"` + test.snapshotRole + `"}]}`
				if _, err := db.Exec(`INSERT INTO comic_chapter_snapshots(uuid,chapter_comic_state_id,actor_id,version_no,reason,snapshot_json,snapshot_hash,created_at) VALUES('019c0000-0000-7000-8000-000000000306',1,1,1,'fixture',?,?,'`+now+`')`, snapshotJSON, strings.Repeat("a", 64)); err != nil {
					t.Fatal(err)
				}
			}
			switch test.durableKind {
			case "export_record_v6":
				snapshotJSON := `{"version":6,"entries":[{"page_role":"body","body_page_no":1}]}`
				if _, err := db.Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,created_at) VALUES('019c0000-0000-7000-8000-000000000307',1,'019c0000-0000-7000-8000-000000000308','project','zip','queued',?,?,'exports/v6.zip',7,?)`, snapshotJSON, strings.Repeat("b", 64), now); err != nil {
					t.Fatal(err)
				}
			case "export_task_v6":
				inputSnapshot := `{"version":1,"kind":"comic_export","parameters":{"version":6,"entries":[{"page_role":"body","body_page_no":1}]}}`
				if _, err := db.Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at) VALUES('019c0000-0000-7000-8000-000000000309',1,'comic_export','019c0000-0000-7000-8000-000000000301',?,'queued','lossy-export-task-v6',?,?)`, inputSnapshot, now, now); err != nil {
					t.Fatal(err)
				}
			case "image_task_v5":
				inputSnapshot := `{"version":5,"kind":"comic_image_generation","page_role":"body"}`
				if _, err := db.Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at) VALUES('019c0000-0000-7000-8000-000000000310',1,'comic_image_generation','019c0000-0000-7000-8000-000000000305',?,'queued','lossy-image-task-v5',?,?)`, inputSnapshot, now, now); err != nil {
					t.Fatal(err)
				}
			case "yolo_workflow_v5":
				inputSnapshot := `{"version":5,"project_uuid":"019c0000-0000-7000-8000-000000000301","picture_book":{"format":"vertical_strip"}}`
				if _, err := db.Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,created_at,updated_at) VALUES('019c0000-0000-7000-8000-000000000311',1,'yolo_project_initialization','Vertical YOLO','queued',1,?,'lossy-yolo-workflow-v5','019c0000-0000-7000-8000-000000000312','model',?,?)`, inputSnapshot, now, now); err != nil {
					t.Fatal(err)
				}
			}
			if err := runner.Down(3); err == nil {
				t.Fatal("down migration accepted page-role history that the old schema cannot represent")
			}
			if !tableHasColumn(t, db, "comic_sections", "page_role") {
				t.Fatal("failed lossy down removed page_role")
			}
		})
	}
}

func TestProjectSetupLifecycleMigrationKeepsExistingProjectsReady(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-setup-lifecycle.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(5); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-28T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'019c0000-0000-7000-8000-000000000101','Existing ready project',1,29,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_picture_book_profiles(project_id,format,aspect_ratio_mode,aspect_width,aspect_height,large_image_minimal_text,created_at) VALUES(1,'classic_picture_book','landscape',4,3,0,?)`, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var setupStatus string
	var profileCount int
	if err := db.QueryRow(`SELECT setup_status FROM projects WHERE id=1`).Scan(&setupStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM project_picture_book_profiles WHERE project_id=1`).Scan(&profileCount); err != nil {
		t.Fatal(err)
	}
	if setupStatus != "ready" || profileCount != 1 {
		t.Fatalf("migrated setup_status=%q profile_count=%d", setupStatus, profileCount)
	}
	for _, column := range []string{"project_name", "generation_language", "overall_style", "format", "field_sources_json", "missing_fields_json"} {
		if !tableHasColumn(t, db, "project_setup_drafts", column) {
			t.Fatalf("project_setup_drafts missing structured draft column %q", column)
		}
	}
	for _, legacyColumn := range []string{"candidate", "candidate_json", "candidate_values"} {
		if tableHasColumn(t, db, "project_setup_drafts", legacyColumn) {
			t.Fatalf("project_setup_drafts retained candidate column %q", legacyColumn)
		}
	}
	if _, err := db.Exec(`DELETE FROM project_picture_book_profiles WHERE project_id=1`); err == nil {
		t.Fatal("setup lifecycle migration left an existing formal profile deletable")
	}
}

func TestChapterChatReferenceMigrationUsesUUIDOnlyInFinalSchema(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "chapter-chat-reference.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if tableHasColumn(t, db, "chat_context_references", "chapter_id") {
		t.Fatal("chapter Reference final schema retained chapter_id")
	}
	var tableSQL string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='chat_context_references'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"resource_type IN ('file', 'premise_asset', 'chapter', 'comic_section')", "resource_type='chapter' AND file_id IS NULL AND premise_asset_id IS NULL AND comic_section_id IS NULL"} {
		if !strings.Contains(tableSQL, required) {
			t.Fatalf("chapter Reference schema missing %q: %s", required, tableSQL)
		}
	}
	if err := runner.Down(6); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_context_references", "chapter_id") {
		t.Fatal("chapter Reference down migration introduced chapter_id")
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_context_references", "chapter_id") {
		t.Fatal("chapter Reference up migration retained chapter_id")
	}
}

func TestChapterChatReferenceMigrationPreservesUUIDWithoutChapterForeignKey(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "chapter-chat-reference-protected-down.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(6); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`ALTER TABLE chat_context_references ADD COLUMN chapter_id INTEGER REFERENCES chapters(id) ON DELETE SET NULL`); err != nil {
		t.Fatal(err)
	}
	const now = "2026-08-28T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000501','Chapter reference',1,28,'` + now + `','` + now + `')`,
		`INSERT INTO chapters(id,uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000502',1,1,1,'chapter-001',1,'Opening','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000503',1,'Chapter thread','idle','01990000-0000-7000-8000-000000000504','model','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(1,'01990000-0000-7000-8000-000000000505',1,1,'user_message','user','Use chapter','{}','` + now + `')`,
		`INSERT INTO chat_context_references(chat_item_id,position,resource_type,resource_uuid,snapshot_json,chapter_id,created_at) VALUES(1,1,'chapter','01990000-0000-7000-8000-000000000502','{"resource_type":"chapter","status":"available"}',1,'` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed chapter reference: %v\n%s", err, statement)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_context_references", "chapter_id") {
		t.Fatal("legacy chapter_id survived UUID-only migration")
	}
	if _, err := db.Exec(`DELETE FROM chapters WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var retainedUUID string
	if err := db.QueryRow(`SELECT resource_uuid FROM chat_context_references WHERE chat_item_id=1`).Scan(&retainedUUID); err != nil {
		t.Fatal(err)
	}
	if retainedUUID != "01990000-0000-7000-8000-000000000502" {
		t.Fatalf("deleted Chapter reference uuid=%q", retainedUUID)
	}
	if err := runner.Down(6); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_context_references", "chapter_id") {
		t.Fatal("chapter Reference compatibility schema contains chapter_id")
	}
	if err := runner.Down(2); err == nil {
		t.Fatal("down migration accepted a Chapter Reference and would lose data")
	}
}

func TestInlineWorkflowAwaitMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "inline-workflow-await.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if !containsTable(tableNames(t, dsn), "workflow_awaits") {
		t.Fatal("up migration missing workflow_awaits")
	}
	if !tableHasColumn(t, db, "chat_threads", "thread_type") {
		t.Fatal("up migration missing chat_threads.thread_type")
	}
	var tableSQL string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_awaits'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"UNIQUE(workflow_id)", "UNIQUE(tool_execution_id)", "status IN ('waiting', 'ready', 'resuming', 'resumed', 'cancelled')"} {
		if !strings.Contains(tableSQL, required) {
			t.Fatalf("workflow_awaits schema missing %q: %s", required, tableSQL)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(10); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "workflow_awaits") {
		t.Fatal("down migration retained workflow_awaits")
	}
	if tableHasColumn(t, db, "chat_threads", "thread_type") {
		t.Fatal("down migration retained chat_threads.thread_type")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestLongRunningChatTurnMigrationBackfillsLegacyStepsAndRoundTrips(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "long-running-chat-turn.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(8); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-26T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000401','Long turn migration',1,26,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000402',1,'Long turn','idle','01990000-0000-7000-8000-000000000403','model','` + now + `','` + now + `')`,
		`INSERT INTO chat_turns(id,uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000404',1,'prompt',1,'Continue','completed','` + now + `','` + now + `')`,
		`INSERT INTO chat_runs(id,uuid,thread_id,turn_id,trigger_type,status,step_count,max_steps,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000405',1,1,'prompt','completed',7,12,'01990000-0000-7000-8000-000000000403','model','` + now + `','` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed legacy run: %v\n%s", err, statement)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var requestCount, maxRequests, noProgressRounds int
	var maxDuration, maxTokens int64
	if err := db.QueryRow(`SELECT model_request_count,max_model_requests,max_active_duration_ms,max_token_units,max_no_progress_rounds FROM chat_runs WHERE id=1`).Scan(&requestCount, &maxRequests, &maxDuration, &maxTokens, &noProgressRounds); err != nil {
		t.Fatal(err)
	}
	if requestCount != 7 || maxRequests != 256 || maxDuration != 7_200_000 || maxTokens != 1_000_000 || noProgressRounds != 2 {
		t.Fatalf("backfilled budgets=%d/%d duration=%d tokens=%d no-progress=%d", requestCount, maxRequests, maxDuration, maxTokens, noProgressRounds)
	}
	if err := runner.Down(8); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_runs", "model_request_count") {
		t.Fatal("down migration retained long-turn budget columns")
	}
	if err := db.QueryRow(`SELECT step_count FROM chat_runs WHERE id=1`).Scan(&requestCount); err != nil || requestCount != 7 {
		t.Fatalf("legacy step count after down=%d err=%v", requestCount, err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestVersionedUserInputRequestMigrationPreservesLegacyAndRejectsLossyDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "versioned-user-input.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(9); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-26T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000301','Input migration',1,25,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000302',1,'Input thread','waiting_for_input','01990000-0000-7000-8000-000000000303','model','` + now + `','` + now + `')`,
		`INSERT INTO chat_turns(id,uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000304',1,'prompt',1,'Choose','waiting_for_input','` + now + `','` + now + `')`,
		`INSERT INTO chat_runs(id,uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000305',1,1,'prompt','waiting_for_input','01990000-0000-7000-8000-000000000303','model','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,metadata_json,created_at) VALUES(1,'01990000-0000-7000-8000-000000000306',1,1,1,1,'user_input_request','assistant','{}','json','completed','{}','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,metadata_json,created_at) VALUES(2,'01990000-0000-7000-8000-000000000311',1,1,1,2,'user_input_request','assistant','{}','json','completed','{}','` + now + `')`,
		`INSERT INTO chat_user_input_requests(id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,input_type,question,options_json,response_json,status,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000307',1,1,1,1,'01990000-0000-7000-8000-000000000308','single_choice','继续吗？','[{"uuid":"01990000-0000-7000-8000-000000000309","label":"继续"},{"uuid":"01990000-0000-7000-8000-000000000310","label":"取消"}]',NULL,'pending','` + now + `','` + now + `')`,
		`INSERT INTO chat_user_input_requests(id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,input_type,question,options_json,response_json,status,answered_at,created_at,updated_at) VALUES(2,'01990000-0000-7000-8000-000000000312',1,1,1,2,'01990000-0000-7000-8000-000000000313','multiple_choice','选择标签？','[{"uuid":"01990000-0000-7000-8000-000000000314","label":"甲"},{"uuid":"01990000-0000-7000-8000-000000000315","label":"乙"}]','{"selected_option_uuids":["01990000-0000-7000-8000-000000000314","01990000-0000-7000-8000-000000000315"],"other_text":"自定义标签"}','answered','` + now + `','` + now + `','` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed legacy input: %v\n%s", err, statement)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "chat_user_input_requests", "input_type") || !tableHasColumn(t, db, "chat_user_input_requests", "schema_version") || !tableHasColumn(t, db, "chat_user_input_requests", "request_json") {
		t.Fatal("up migration did not establish a single versioned request source")
	}
	var schemaVersion, requestJSON, status string
	var responseJSON sql.NullString
	if err := db.QueryRow(`SELECT schema_version,request_json,response_json,status FROM chat_user_input_requests WHERE id=1`).Scan(&schemaVersion, &requestJSON, &responseJSON, &status); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != "legacy_choice_v1" || !strings.Contains(requestJSON, `"input_type":"single_choice"`) || !strings.Contains(requestJSON, `"question":"继续吗？"`) || responseJSON.Valid || status != "pending" {
		t.Fatalf("legacy pending single choice was not preserved: schema=%s request=%s response=%+v status=%s", schemaVersion, requestJSON, responseJSON, status)
	}
	if err := db.QueryRow(`SELECT schema_version,request_json,response_json,status FROM chat_user_input_requests WHERE id=2`).Scan(&schemaVersion, &requestJSON, &responseJSON, &status); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != "legacy_choice_v1" || !strings.Contains(requestJSON, `"input_type":"multiple_choice"`) || !responseJSON.Valid || !strings.Contains(responseJSON.String, `"selected_option_uuids"`) || !strings.Contains(responseJSON.String, `"other_text":"自定义标签"`) || status != "answered" {
		t.Fatalf("legacy answered multiple choice and Other were not preserved: schema=%s request=%s response=%+v status=%s", schemaVersion, requestJSON, responseJSON, status)
	}
	if err := runner.Down(9); err != nil {
		t.Fatal(err)
	}
	var inputType, question, optionsJSON string
	if err := db.QueryRow(`SELECT input_type,question,options_json FROM chat_user_input_requests WHERE id=1`).Scan(&inputType, &question, &optionsJSON); err != nil {
		t.Fatal(err)
	}
	if inputType != "single_choice" || question != "继续吗？" || !strings.Contains(optionsJSON, "继续") {
		t.Fatalf("legacy round trip failed: %s %s %s", inputType, question, optionsJSON)
	}
	if err := db.QueryRow(`SELECT input_type,question,options_json FROM chat_user_input_requests WHERE id=2`).Scan(&inputType, &question, &optionsJSON); err != nil {
		t.Fatal(err)
	}
	if inputType != "multiple_choice" || question != "选择标签？" || !strings.Contains(optionsJSON, "甲") {
		t.Fatalf("legacy multiple-choice round trip failed: %s %s %s", inputType, question, optionsJSON)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,content_format,status,metadata_json,created_at) VALUES(3,'01990000-0000-7000-8000-000000000316',1,1,1,3,'user_input_request','assistant','{}','json','completed','{}',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_user_input_requests(id,uuid,thread_id,run_id,turn_id,item_id,tool_call_uuid,schema_version,request_json,status,created_at,updated_at) VALUES(3,'01990000-0000-7000-8000-000000000317',1,1,1,3,'01990000-0000-7000-8000-000000000318','codex_questions_v1','{"questions":[]}','pending',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(9); err == nil {
		t.Fatal("down migration accepted a v4 multi-question row and would lose data")
	}
}

func TestPictureBookProfileMigrationBackfillsPreProfileProjectsAndIsImmutable(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "picture-book-profile.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(16); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "project_picture_book_profiles") {
		t.Fatal("picture book profile down migration retained its table")
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-12T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01900000-0000-7000-8000-000000000101','Pre-profile project',1,18,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var format, mode string
	var width, height int
	if err := db.QueryRow(`SELECT format,aspect_ratio_mode,aspect_width,aspect_height FROM project_picture_book_profiles WHERE project_id=1`).Scan(&format, &mode, &width, &height); err != nil {
		t.Fatal(err)
	}
	if format != "vertical_strip" || mode != "fixed" || width != 1 || height != 3 {
		t.Fatalf("backfilled profile=%s %s %d:%d", format, mode, width, height)
	}
	if _, err := db.Exec(`UPDATE project_picture_book_profiles SET format='comic_story' WHERE project_id=1`); err == nil {
		t.Fatal("immutable profile accepted an update")
	}
	if _, err := db.Exec(`INSERT INTO project_picture_book_profiles(project_id,format,aspect_ratio_mode,aspect_width,aspect_height,created_at) VALUES(1,'vertical_strip','fixed',1,3,?)`, now); err == nil {
		t.Fatal("project accepted a second picture book profile")
	}
	if _, err := db.Exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(2,'01900000-0000-7000-8000-000000000102','Invalid profile',1,19,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_picture_book_profiles(project_id,format,aspect_ratio_mode,aspect_width,aspect_height,large_image_minimal_text,interaction_mode,created_at) VALUES(2,'classic_picture_book','landscape',4,3,0,'find_it',?)`, now); err == nil {
		t.Fatal("cross-field constraint accepted irrelevant interaction_mode")
	}
	if _, err := db.Exec(`INSERT INTO project_picture_book_profiles(project_id,format,aspect_ratio_mode,aspect_width,aspect_height,interaction_mode,created_at) VALUES(2,'interactive_picture_book','square',1,1,'find_it',?)`, now); err == nil {
		t.Fatal("interactive profile accepted an irrelevant aspect ratio")
	}
}

func TestProjectModelSettingsMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "project-model-settings.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "project_model_settings") {
		t.Fatal("model settings table was not created")
	}
	for _, table := range []string{"task_runs", "production_task_runs", "chat_threads", "chat_runs", "workflows"} {
		if !tableHasColumn(t, db, table, "model_source") {
			t.Fatalf("%s missing model_source", table)
		}
		var defaultValue string
		if err := db.QueryRow(`SELECT dflt_value FROM pragma_table_info('` + table + `') WHERE name='model_source'`).Scan(&defaultValue); err != nil || defaultValue != "'legacy_frozen'" {
			t.Fatalf("%s model_source default=%q err=%v", table, defaultValue, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(18); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "project_model_settings") {
		t.Fatal("down migration retained project_model_settings")
	}
	for _, table := range []string{"task_runs", "production_task_runs", "chat_threads", "chat_runs", "workflows"} {
		if tableHasColumn(t, db, table, "model_source") {
			t.Fatalf("down migration retained %s.model_source", table)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectChatContextReferenceMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "chat-image-references.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(11); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const (
		now                 = "2026-08-25T00:00:00Z"
		projectUUID         = "01990000-0000-7000-8000-000000000101"
		actorUUID           = "01990000-0000-7000-8000-000000000102"
		providerUUID        = "01990000-0000-7000-8000-000000000103"
		assetUUID           = "01990000-0000-7000-8000-000000000104"
		assetVariantUUID    = "01990000-0000-7000-8000-000000000105"
		chapterUUID         = "01990000-0000-7000-8000-000000000106"
		comicStateUUID      = "01990000-0000-7000-8000-000000000107"
		sectionUUID         = "01990000-0000-7000-8000-000000000108"
		storyboardUUID      = "01990000-0000-7000-8000-000000000109"
		comicImageUUID      = "01990000-0000-7000-8000-000000000110"
		domainImageFileUUID = "01990000-0000-7000-8000-000000000111"
		uploadFileUUID      = "01990000-0000-7000-8000-000000000112"
		missingAssetUUID    = "01990000-0000-7000-8000-000000000113"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','References',1,24,'` + now + `','` + now + `')`,
		`INSERT INTO actors(id,uuid,name,kind,created_at,updated_at) VALUES(1,'` + actorUUID + `','Tester','local_user','` + now + `','` + now + `')`,
		`INSERT INTO file_objects(id,uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at) VALUES(1,'01990000-0000-7000-8000-000000000114',1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','objects/domain.png','image/png','png',2048,640,480,'ready','` + now + `','` + now + `')`,
		`INSERT INTO file_objects(id,uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,width,height,state,created_at,verified_at) VALUES(2,'01990000-0000-7000-8000-000000000115',1,'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','objects/upload.png','image/png','png',1024,320,240,'ready','` + now + `','` + now + `')`,
		`INSERT INTO files(id,uuid,project_id,file_object_id,kind,purpose,original_filename,display_name,source_type,metadata_json,created_at) VALUES(1,'` + domainImageFileUUID + `',1,1,'image','premise_asset','domain.png','Domain image','imported','{}','` + now + `')`,
		`INSERT INTO files(id,uuid,project_id,file_object_id,kind,purpose,original_filename,display_name,source_type,metadata_json,created_at) VALUES(2,'` + uploadFileUUID + `',1,2,'image','chat_reference','upload.png','Upload image','imported','{}','` + now + `')`,
		`INSERT INTO premise_assets(id,uuid,project_id,actor_id,asset_type,title,summary,revision,created_at,updated_at) VALUES(1,'` + assetUUID + `',1,1,'character','Mira','A compact migrated summary',3,'` + now + `','` + now + `')`,
		`INSERT INTO premise_asset_tags(id,premise_asset_id,tag,created_at) VALUES(1,1,'hero','` + now + `')`,
		`INSERT INTO premise_asset_variants(id,uuid,premise_asset_id,file_id,version_no,source_type,created_at) VALUES(1,'` + assetVariantUUID + `',1,1,1,'manual','` + now + `')`,
		`UPDATE premise_assets SET current_variant_id=1 WHERE id=1`,
		`INSERT INTO chapters(id,uuid,project_id,volume_no,chapter_no,chapter_code,sort_order,title,created_at,updated_at) VALUES(1,'` + chapterUUID + `',1,1,1,'chapter-001',1,'Opening','` + now + `','` + now + `')`,
		`INSERT INTO chapter_comic_states(id,uuid,chapter_id,status,revision,created_at,updated_at) VALUES(1,'` + comicStateUUID + `',1,'ready',2,'` + now + `','` + now + `')`,
		`INSERT INTO comic_sections(id,uuid,chapter_comic_state_id,actor_id,section_no,title,description_md,revision,created_at,updated_at) VALUES(1,'` + sectionUUID + `',1,1,1,'Arrival','Mira arrives.',4,'` + now + `','` + now + `')`,
		`INSERT INTO comic_storyboard_variants(id,uuid,comic_section_id,actor_id,version_no,content_md,source_type,created_at) VALUES(1,'` + storyboardUUID + `',1,1,1,'Panel one','manual','` + now + `')`,
		`INSERT INTO comic_image_variants(id,uuid,comic_section_id,file_id,actor_id,version_no,source_type,input_snapshot,created_at) VALUES(1,'` + comicImageUUID + `',1,1,1,1,'manual','{}','` + now + `')`,
		`UPDATE comic_sections SET current_storyboard_variant_id=1,current_image_variant_id=1 WHERE id=1`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,scope,scene,subject_uuid,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000121',1,'Asset thread','idle','` + providerUUID + `','test-model','premise','asset_reference','` + assetUUID + `','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,scope,scene,subject_uuid,created_at,updated_at) VALUES(2,'01990000-0000-7000-8000-000000000122',1,'Comic thread','idle','` + providerUUID + `','test-model','project','asset_reference','` + sectionUUID + `','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,scope,scene,subject_uuid,created_at,updated_at) VALUES(3,'01990000-0000-7000-8000-000000000123',1,'Missing thread','idle','` + providerUUID + `','test-model','premise','asset_reference','` + missingAssetUUID + `','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,scope,scene,subject_uuid,created_at,updated_at) VALUES(4,'01990000-0000-7000-8000-000000000124',1,'Follow-up thread','idle','` + providerUUID + `','test-model','project','','','` + now + `','` + now + `')`,
		`INSERT INTO chat_turns(id,uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000131',1,'prompt',1,'Use Mira','completed','` + now + `','` + now + `')`,
		`INSERT INTO chat_runs(id,uuid,thread_id,turn_id,trigger_type,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000132',1,1,'prompt','completed','` + providerUUID + `','test-model','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,turn_id,run_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(1,'01990000-0000-7000-8000-000000000141',1,1,1,1,'user_message','user','Use Mira','{}','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(2,'01990000-0000-7000-8000-000000000142',2,1,'user_message','user','Use the section','{}','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(3,'01990000-0000-7000-8000-000000000143',3,1,'user_message','user','Use missing asset','{}','` + now + `')`,
		`INSERT INTO chat_follow_ups(id,uuid,thread_id,input_text,position,status,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000151',4,'Use upload',1,'queued','` + now + `','` + now + `')`,
		`INSERT INTO chat_item_file_references(id,uuid,chat_item_id,file_id,position,created_at) VALUES(1,'01990000-0000-7000-8000-000000000161',1,2,1,'` + now + `')`,
		`INSERT INTO chat_item_file_references(id,uuid,chat_item_id,file_id,position,created_at) VALUES(2,'01990000-0000-7000-8000-000000000163',1,2,2,'` + now + `')`,
		`INSERT INTO chat_follow_up_file_references(id,uuid,follow_up_id,file_id,position,created_at) VALUES(1,'01990000-0000-7000-8000-000000000162',1,2,1,'` + now + `')`,
		`INSERT INTO chat_follow_up_file_references(id,uuid,follow_up_id,file_id,position,created_at) VALUES(2,'01990000-0000-7000-8000-000000000164',1,2,2,'` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed legacy chat references: %v\n%s", err, statement)
		}
	}

	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if !containsTable(tableNames(t, dsn), "chat_context_references") {
		t.Fatal("up migration missing chat_context_references")
	}
	for _, table := range []string{"chat_item_file_references", "chat_follow_up_file_references"} {
		if containsTable(tableNames(t, dsn), table) {
			t.Fatalf("up migration retained legacy %s", table)
		}
	}
	for _, column := range []string{"scope", "scene", "subject_uuid"} {
		if tableHasColumn(t, db, "chat_threads", column) {
			t.Fatalf("up migration retained chat_threads.%s", column)
		}
	}

	type migratedReference struct {
		position     int
		resourceType string
		resourceUUID string
		status       string
		targetID     sql.NullInt64
		imageFileID  sql.NullInt64
	}
	readReference := func(ownerColumn string, ownerID int64, position int) migratedReference {
		t.Helper()
		var ref migratedReference
		targetExpression := "COALESCE(file_id,premise_asset_id,comic_section_id)"
		err := db.QueryRow(`SELECT position,resource_type,resource_uuid,json_extract(snapshot_json,'$.status'),`+targetExpression+`,image_file_id FROM chat_context_references WHERE `+ownerColumn+`=? AND position=?`, ownerID, position).Scan(
			&ref.position, &ref.resourceType, &ref.resourceUUID, &ref.status, &ref.targetID, &ref.imageFileID,
		)
		if err != nil {
			t.Fatalf("read migrated reference %s=%d position=%d: %v", ownerColumn, ownerID, position, err)
		}
		return ref
	}
	assetRef := readReference("chat_item_id", 1, 1)
	if assetRef.resourceType != "premise_asset" || assetRef.resourceUUID != assetUUID || assetRef.status != "available" || assetRef.targetID.Int64 != 1 || assetRef.imageFileID.Int64 != 1 {
		t.Fatalf("asset reference = %+v", assetRef)
	}
	uploadRef := readReference("chat_item_id", 1, 2)
	if uploadRef.resourceType != "file" || uploadRef.resourceUUID != uploadFileUUID || uploadRef.targetID.Int64 != 2 || uploadRef.imageFileID.Int64 != 2 {
		t.Fatalf("upload reference = %+v", uploadRef)
	}
	comicRef := readReference("chat_item_id", 2, 1)
	if comicRef.resourceType != "comic_section" || comicRef.resourceUUID != sectionUUID || comicRef.status != "available" || comicRef.targetID.Int64 != 1 || comicRef.imageFileID.Int64 != 1 {
		t.Fatalf("comic reference = %+v", comicRef)
	}
	missingRef := readReference("chat_item_id", 3, 1)
	if missingRef.resourceType != "premise_asset" || missingRef.resourceUUID != missingAssetUUID || missingRef.status != "unavailable" || missingRef.targetID.Valid || missingRef.imageFileID.Valid {
		t.Fatalf("missing reference = %+v", missingRef)
	}
	followUpRef := readReference("follow_up_id", 1, 1)
	if followUpRef.resourceType != "file" || followUpRef.resourceUUID != uploadFileUUID || followUpRef.targetID.Int64 != 2 {
		t.Fatalf("follow-up reference = %+v", followUpRef)
	}
	var legacyScope, legacyScene, legacySubject string
	if err := db.QueryRow(`SELECT json_extract(metadata_json,'$.legacy_thread_context.scope'),json_extract(metadata_json,'$.legacy_thread_context.scene'),json_extract(metadata_json,'$.legacy_thread_context.subject_uuid') FROM chat_items WHERE id=1`).Scan(&legacyScope, &legacyScene, &legacySubject); err != nil {
		t.Fatal(err)
	}
	if legacyScope != "premise" || legacyScene != "asset_reference" || legacySubject != assetUUID {
		t.Fatalf("legacy context = %q %q %q", legacyScope, legacyScene, legacySubject)
	}

	if err := runner.Down(11); err != nil {
		t.Fatal(err)
	}
	if containsTable(tableNames(t, dsn), "chat_context_references") {
		t.Fatal("down migration retained chat_context_references")
	}
	for _, table := range []string{"chat_item_file_references", "chat_follow_up_file_references"} {
		if !containsTable(tableNames(t, dsn), table) {
			t.Fatalf("down migration missing legacy %s", table)
		}
	}
	var scope, scene, subject string
	if err := db.QueryRow(`SELECT scope,scene,subject_uuid FROM chat_threads WHERE id=1`).Scan(&scope, &scene, &subject); err != nil {
		t.Fatal(err)
	}
	if scope != "premise" || scene != "asset_reference" || subject != assetUUID {
		t.Fatalf("restored asset thread = %q %q %q", scope, scene, subject)
	}
	var itemFiles, followUpFiles int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chat_item_file_references WHERE chat_item_id=1 AND file_id=2 AND position=1`).Scan(&itemFiles); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM chat_follow_up_file_references WHERE follow_up_id=1 AND file_id=2 AND position=1`).Scan(&followUpFiles); err != nil {
		t.Fatal(err)
	}
	if itemFiles != 1 || followUpFiles != 1 {
		t.Fatalf("restored file references item=%d follow_up=%d", itemFiles, followUpFiles)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectChatContextReferenceDownRejectsLossyData(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "chat-reference-protected-down.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-25T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000201','Protected down',1,24,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000202',1,'Protected','idle','01990000-0000-7000-8000-000000000203','test-model','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(1,'01990000-0000-7000-8000-000000000204',1,1,'user_message','user','Five files','{}','` + now + `')`,
	}
	for position := 1; position <= 5; position++ {
		resourceUUID := "01990000-0000-7000-8000-00000000020" + string(rune('4'+position))
		statements = append(statements, `INSERT INTO chat_context_references(chat_item_id,position,resource_type,resource_uuid,snapshot_json,created_at) VALUES(1,`+string(rune('0'+position))+`,'file','`+resourceUUID+`','{"resource_type":"file","status":"unavailable"}','`+now+`')`)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed protected down: %v\n%s", err, statement)
		}
	}
	if err := runner.Down(11); err == nil {
		t.Fatal("down migration accepted five file references and would lose data")
	}
}

func TestProjectChatContextReferenceDownRejectsTurnScopedDomainReference(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "chat-reference-turn-scoped-down.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const now = "2026-08-25T00:00:00Z"
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000221','Turn scoped down',1,24,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,created_at,updated_at) VALUES(1,'01990000-0000-7000-8000-000000000222',1,'Turn scoped','idle','01990000-0000-7000-8000-000000000223','test-model','` + now + `','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(1,'01990000-0000-7000-8000-000000000224',1,1,'user_message','user','With reference','{}','` + now + `')`,
		`INSERT INTO chat_items(id,uuid,thread_id,sequence,item_type,role,content,metadata_json,created_at) VALUES(2,'01990000-0000-7000-8000-000000000225',1,2,'user_message','user','Without reference','{}','` + now + `')`,
		`INSERT INTO chat_context_references(chat_item_id,position,resource_type,resource_uuid,snapshot_json,created_at) VALUES(1,1,'premise_asset','01990000-0000-7000-8000-000000000226','{"resource_type":"premise_asset","status":"unavailable"}','` + now + `')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed turn-scoped protected down: %v\n%s", err, statement)
		}
	}
	if err := runner.Down(11); err == nil {
		t.Fatal("down migration accepted a domain Reference that only applies to one Turn")
	}
}

func TestAssetStoreMigrationDownRemovesSchemaCleanly(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "asset-store.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	// Every later project migration depends on Asset Store, so roll back
	// through the Asset Store migration before asserting its down contract.
	if err := runner.Down(32); err != nil {
		t.Fatal(err)
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	tables := tableNames(t, dsn)
	for _, table := range []string{"upload_stashed", "file_objects", "files", "integrity_scans", "asset_maintenance_runs"} {
		if containsTable(tables, table) {
			t.Fatalf("down migration retained %s: %v", table, tables)
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("PRAGMA table_info(story_source_items)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "file_id" {
			t.Fatal("down migration retained story_source_items.file_id")
		}
	}
}

func TestComicSectionPremiseMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "section-premise.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !tableHasColumn(t, db, "comic_image_generations", "premise_file_id") || !tableHasColumn(t, db, "comic_image_generations", "premise_metadata") {
		t.Fatal("section premise migration did not add both generation columns")
	}
	var defaultMetadata string
	if err := db.QueryRow(`SELECT dflt_value FROM pragma_table_info('comic_image_generations') WHERE name='premise_metadata'`).Scan(&defaultMetadata); err != nil || defaultMetadata != "'{}'" {
		t.Fatalf("premise_metadata default=%q err=%v", defaultMetadata, err)
	}
	var fkTable, fromColumn, toColumn string
	if err := db.QueryRow(`SELECT "table","from","to" FROM pragma_foreign_key_list('comic_image_generations') WHERE "from"='premise_file_id'`).Scan(&fkTable, &fromColumn, &toColumn); err != nil || fkTable != "files" || fromColumn != "premise_file_id" || toColumn != "id" {
		t.Fatalf("premise FK=%s %s->%s err=%v", fkTable, fromColumn, toColumn, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(22); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if tableHasColumn(t, db, "comic_image_generations", "premise_file_id") || tableHasColumn(t, db, "comic_image_generations", "premise_metadata") {
		t.Fatal("section premise down migration retained generation columns")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestComicImageWorkflowMigrationPreservesExistingYoloGraph(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-workflow.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(20); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	const (
		projectUUID  = "01900000-0000-7000-8000-000000000001"
		providerUUID = "01900000-0000-7000-8000-000000000002"
		threadUUID   = "01900000-0000-7000-8000-000000000003"
		workflowUUID = "01900000-0000-7000-8000-000000000004"
		stepUUID     = "01900000-0000-7000-8000-000000000005"
		eventUUID    = "01900000-0000-7000-8000-000000000006"
		logUUID      = "01900000-0000-7000-8000-000000000007"
		now          = "2026-08-10T00:00:00Z"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','Migration fixture',1,14,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,next_turn_sequence,next_item_sequence,next_event_sequence,scope,scene,subject_uuid,created_at,updated_at) VALUES(1,'` + threadUUID + `',1,'Yolo fixture','completed','` + providerUUID + `','model',1,1,1,'project','','','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,current_step_key,created_at,updated_at) VALUES(1,'` + workflowUUID + `',1,1,'yolo_project_initialization','Yolo fixture','completed',1,'{}','migration-yolo','` + providerUUID + `','model','project_initialization','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(1,'` + stepUUID + `',1,'project_initialization',1,'completed','migration-yolo-step','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_events(id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(1,'` + eventUUID + `',1,1,1,'workflow_completed','{}','` + now + `')`,
		`INSERT INTO llm_logs(id,uuid,project_id,workflow_id,workflow_step_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,created_at,completed_at,request_payload,response) VALUES(1,'` + logUUID + `',1,1,1,'workflow','migration_fixture','text',1,'` + providerUUID + `','openai_compatible','model','completed','` + now + `','` + now + `','{}','{}')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	assertGraph := func(stage string) {
		t.Helper()
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		for table, uuid := range map[string]string{"workflows": workflowUUID, "workflow_steps": stepUUID, "workflow_events": eventUUID, "llm_logs": logUUID} {
			var count int
			if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE uuid=?", uuid).Scan(&count); err != nil || count != 1 {
				t.Fatalf("%s %s count=%d err=%v", stage, table, count, err)
			}
		}
		rows, err := db.Query("PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatalf("%s left a foreign key violation", stage)
		}
	}

	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertGraph("up")
	if err := runner.Down(20); err != nil {
		t.Fatal(err)
	}
	assertGraph("down")
}

func TestPremiseComicMigrationDownKeepsAssetStore(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "premise-comic.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	// Roll back Prompt catalog, Premise Chat parity, project language and
	// Chat/Yolo first, then Premise/Comic.
	if err := runner.Down(30); err != nil {
		t.Fatal(err)
	}
	if err := runner.Close(); err != nil {
		t.Fatal(err)
	}
	tables := tableNames(t, dsn)
	for _, table := range []string{"premise_profiles", "premise_assets", "comic_sections", "comic_exports", "production_task_runs"} {
		if containsTable(tables, table) {
			t.Fatalf("Goal 04 down migration retained %s: %v", table, tables)
		}
	}
	for _, table := range []string{"upload_stashed", "file_objects", "files", "integrity_scans", "asset_maintenance_runs"} {
		if !containsTable(tables, table) {
			t.Fatalf("Goal 04 down migration removed Asset Store table %s: %v", table, tables)
		}
	}
}

func TestPremiseChatParityMigrationPreservesExistingRowsAndForeignKeys(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "premise-chat-parity.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	// Roll back unified logs and Prompt catalog first, then Premise Chat parity.
	if err := runner.Down(27); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := "2026-08-09T00:00:00Z"
	projectUUID := "019fe3ec-d504-712c-a660-70761932ca66"
	actorUUID := "019fe3ec-d504-712c-a660-70761932ca67"
	providerUUID := "019fe3ec-d504-712c-a660-70761932ca68"
	if _, err := db.Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?,?,1,1,?,?)`, projectUUID, "Parity", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO actors(uuid,name,kind,created_at,updated_at) VALUES(?,?,'local_user',?,?)`, actorUUID, "Owner", now, now); err != nil {
		t.Fatal(err)
	}
	var projectID, actorID int64
	if err := db.QueryRow(`SELECT id FROM projects WHERE uuid=?`, projectUUID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM actors WHERE uuid=?`, actorUUID).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	sourceUUID := "019fe3ec-d504-712c-a660-70761932ca69"
	threadUUID := "019fe3ec-d504-712c-a660-70761932ca70"
	promptUUID := "019fe3ec-d504-712c-a660-70761932ca71"
	taskUUID := "019fe3ec-d504-712c-a660-70761932ca72"
	eventUUID := "019fe3ec-d504-712c-a660-70761932ca73"
	if _, err := db.Exec(`INSERT INTO premise_sources(uuid,project_id,actor_id,source_type,source_text,style_snapshot,parameters_json,created_at) VALUES(?,?,?,'manual','Existing premise','ink','{}',?)`, sourceUUID, projectID, actorID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,'Existing chat','idle',?,'model',1,1,1,?,?)`, threadUUID, projectID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_prompt_versions(uuid,project_id,actor_id,prompt_group,prompt_key,version_no,prompt,prompt_hash,source_type,created_at) VALUES(?,?,?,'story','outline',1,'Existing prompt','0000000000000000000000000000000000000000000000000000000000000000','manual_edit',?)`, promptUUID, projectID, actorID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO production_task_runs(uuid,project_id,kind,resource_uuid,input_snapshot,status,idempotency_key,created_at,updated_at) VALUES(?,?,'comic_export',?,'{}','completed','existing-export',?,?)`, taskUUID, projectID, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var taskID int64
	if err := db.QueryRow(`SELECT id FROM production_task_runs WHERE uuid=?`, taskUUID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO production_task_events(uuid,production_task_run_id,sequence,event_type,payload,created_at) VALUES(?,?,1,'task_completed','{}',?)`, eventUUID, taskID, now); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	var ignored any
	var revision int64
	if err := db.QueryRow(`SELECT ignored_at,revision FROM premise_sources WHERE uuid=?`, sourceUUID).Scan(&ignored, &revision); err != nil || ignored != nil || revision != 0 {
		t.Fatalf("source migration ignored=%v revision=%d err=%v", ignored, revision, err)
	}
	var title, status string
	if err := db.QueryRow(`SELECT title,status FROM chat_threads WHERE uuid=?`, threadUUID).Scan(&title, &status); err != nil || title != "Existing chat" || status != "idle" {
		t.Fatalf("thread migration title=%q status=%q err=%v", title, status, err)
	}
	for _, column := range []string{"scope", "scene", "subject_uuid"} {
		if tableHasColumn(t, db, "chat_threads", column) {
			t.Fatalf("latest thread schema retained %s", column)
		}
	}
	for table, uuid := range map[string]string{"project_prompt_versions": promptUUID, "production_task_runs": taskUUID, "production_task_events": eventUUID} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE uuid=?`, uuid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("premise chat parity migration left a foreign key violation")
	}
}

func TestStoryPromptWorkflowMigrationPreservesAuditAndAgentPromptHistory(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "story-prompt-workflows.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(25); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-09T00:00:00Z"
	projectUUID := "019fe3ec-d504-712c-a660-70761932cb01"
	actorUUID := "019fe3ec-d504-712c-a660-70761932cb02"
	providerUUID := "019fe3ec-d504-712c-a660-70761932cb03"
	taskUUID := "019fe3ec-d504-712c-a660-70761932cb04"
	eventUUID := "019fe3ec-d504-712c-a660-70761932cb05"
	threadUUID := "019fe3ec-d504-712c-a660-70761932cb06"
	runUUID := "019fe3ec-d504-712c-a660-70761932cb07"
	agentEventUUID := "019fe3ec-d504-712c-a660-70761932cb08"
	logUUID := "019fe3ec-d504-712c-a660-70761932cb09"
	promptUUID := "019fe3ec-d504-712c-a660-70761932cb10"
	if _, err := db.Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?, 'Workflow migration', 1, 1, ?, ?)`, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO actors(uuid,name,kind,created_at,updated_at) VALUES(?, 'Owner', 'local_user', ?, ?)`, actorUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var projectID, actorID int64
	if err := db.QueryRow(`SELECT id FROM projects WHERE uuid=?`, projectUUID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM actors WHERE uuid=?`, actorUUID).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_prompt_versions(uuid,project_id,actor_id,prompt_group,prompt_key,version_no,prompt,prompt_hash,source_type,created_at) VALUES(?,?,?,'premise','agent.project_assistant',1,'Preserved agent prompt','0000000000000000000000000000000000000000000000000000000000000000','manual_edit',?)`, promptUUID, projectID, actorID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'story_chapter_generation',?,1,'{}','completed','existing-chapter-task',?,'model',100,1,3,?,?)`, taskUUID, projectID, projectUUID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var taskID int64
	if err := db.QueryRow(`SELECT id FROM task_runs WHERE uuid=?`, taskUUID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO task_events(uuid,task_run_id,sequence,event_type,payload,created_at) VALUES(?,?,1,'task_completed','{}',?)`, eventUUID, taskID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_threads(uuid,project_id,kind,subject_type,subject_uuid,status,provider_uuid,model,created_at,updated_at) VALUES(?,?,'story_generation','chapter',?,'completed',?,'model',?,?)`, threadUUID, projectID, projectUUID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var threadID int64
	if err := db.QueryRow(`SELECT id FROM agent_threads WHERE uuid=?`, threadUUID).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_runs(uuid,agent_thread_id,task_run_id,trigger_type,status,created_at,updated_at) VALUES(?,?,?,'job_step','completed',?,?)`, runUUID, threadID, taskID, now, now); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(`SELECT id FROM agent_runs WHERE uuid=?`, runUUID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_events(uuid,agent_thread_id,agent_run_id,sequence,event_type,payload,created_at) VALUES(?,?,?,1,'run_completed','{}',?)`, agentEventUUID, threadID, runID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO llm_logs(uuid,project_id,task_run_id,agent_thread_id,agent_run_id,scenario,provider_uuid,provider_type,model,status,created_at) VALUES(?,?,?,?,?,'story_chapter_generation',?,'openai_compatible','model','completed',?)`, logUUID, projectID, taskID, threadID, runID, providerUUID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	assertMigrationRow := func(table, uuid string) {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE uuid=?`, uuid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	for table, uuid := range map[string]string{"task_runs": taskUUID, "task_events": eventUUID, "agent_runs": runUUID, "agent_events": agentEventUUID, "llm_logs": logUUID} {
		assertMigrationRow(table, uuid)
	}
	var promptGroup, promptKey string
	if err := db.QueryRow(`SELECT prompt_group,prompt_key FROM project_prompt_versions WHERE uuid=?`, promptUUID).Scan(&promptGroup, &promptKey); err != nil || promptGroup != "agent" || promptKey != "project_assistant" {
		t.Fatalf("up prompt identity=%s/%s err=%v", promptGroup, promptKey, err)
	}
	workflowTaskUUID := "019fe3ec-d504-712c-a660-70761932cb11"
	if _, err := db.Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,max_attempts,created_at,updated_at) VALUES(?,?,'story_profile_generation',?,2,'{}','completed','workflow-only',?,'model',3,?,?)`, workflowTaskUUID, projectID, projectUUID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runner.Down(25); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	for table, uuid := range map[string]string{"task_runs": taskUUID, "task_events": eventUUID, "agent_runs": runUUID, "agent_events": agentEventUUID, "llm_logs": logUUID} {
		assertMigrationRow(table, uuid)
	}
	var workflowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_runs WHERE uuid=?`, workflowTaskUUID).Scan(&workflowCount); err != nil || workflowCount != 0 {
		t.Fatalf("workflow task survived rollback: count=%d err=%v", workflowCount, err)
	}
	if err := db.QueryRow(`SELECT prompt_group,prompt_key FROM project_prompt_versions WHERE uuid=?`, promptUUID).Scan(&promptGroup, &promptKey); err != nil || promptGroup != "premise" || promptKey != "agent.project_assistant" {
		t.Fatalf("down prompt identity=%s/%s err=%v", promptGroup, promptKey, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("story workflow migration rollback left a foreign key violation")
	}
	rows.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT prompt_group,prompt_key FROM project_prompt_versions WHERE uuid=?`, promptUUID).Scan(&promptGroup, &promptKey); err != nil || promptGroup != "agent" || promptKey != "project_assistant" {
		t.Fatalf("re-up prompt identity=%s/%s err=%v", promptGroup, promptKey, err)
	}
}

func TestUnifiedAICallLogMigrationPreservesStoryAndChatRows(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "unified-ai-call-logs.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(24); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-10T00:00:00Z"
	projectUUID := "019fe98b-0000-7000-8000-000000000001"
	providerUUID := "019fe98b-0000-7000-8000-000000000002"
	taskUUID := "019fe98b-0000-7000-8000-000000000003"
	storyLogUUID := "019fe98b-0000-7000-8000-000000000004"
	threadUUID := "019fe98b-0000-7000-8000-000000000005"
	turnUUID := "019fe98b-0000-7000-8000-000000000006"
	runUUID := "019fe98b-0000-7000-8000-000000000007"
	chatLogUUID := "019fe98b-0000-7000-8000-000000000008"
	if _, err := db.Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?,'Logs',1,1,?,?)`, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := db.QueryRow(`SELECT id FROM projects WHERE uuid=?`, projectUUID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'story_chapter_generation',?,1,'{}','completed','story-log',?,'text-model',100,1,3,?,?)`, taskUUID, projectID, projectUUID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var taskID int64
	if err := db.QueryRow(`SELECT id FROM task_runs WHERE uuid=?`, taskUUID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO llm_logs(uuid,project_id,task_run_id,scenario,provider_uuid,provider_type,model,status,created_at,completed_at) VALUES(?,?,?,'story_chapter_generation',?,'openai_compatible','text-model','completed',?,?)`, storyLogUUID, projectID, taskID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_threads(uuid,project_id,title,status,provider_uuid,model,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(?,?,'Chat','idle',?,'chat-model',2,1,1,?,?)`, threadUUID, projectID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var threadID int64
	if err := db.QueryRow(`SELECT id FROM chat_threads WHERE uuid=?`, threadUUID).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_turns(uuid,thread_id,source_type,queue_sequence,input_text,status,created_at,updated_at) VALUES(?,?,'prompt',1,'hello','completed',?,?)`, turnUUID, threadID, now, now); err != nil {
		t.Fatal(err)
	}
	var turnID int64
	if err := db.QueryRow(`SELECT id FROM chat_turns WHERE uuid=?`, turnUUID).Scan(&turnID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_runs(uuid,thread_id,turn_id,trigger_type,status,step_count,max_steps,provider_uuid,model,context_bytes,created_at,updated_at) VALUES(?,?,?,'prompt','completed',1,12,?,'chat-model',10,?,?)`, runUUID, threadID, turnID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(`SELECT id FROM chat_runs WHERE uuid=?`, runUUID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_model_calls(uuid,project_id,thread_id,run_id,provider_uuid,model,status,input_tokens,output_tokens,duration_ms,finish_reason,error_code,created_at) VALUES(?,?,?,?,?,'chat-model','completed',5,2,100,'stop','',?)`, chatLogUUID, projectID, threadID, runID, providerUUID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	for uuid, source := range map[string]string{storyLogUUID: "story_generation", chatLogUUID: "project_chat"} {
		var actual string
		if err := db.QueryRow(`SELECT source_type FROM llm_logs WHERE uuid=?`, uuid).Scan(&actual); err != nil || actual != source {
			t.Fatalf("log %s source=%q err=%v", uuid, actual, err)
		}
	}
	var oldTable int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_model_calls'`).Scan(&oldTable); err != nil || oldTable != 0 {
		t.Fatalf("agent_model_calls survived up migration: count=%d err=%v", oldTable, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatal("unified AI call log migration left a foreign key violation")
	}
	rows.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(24); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for table, uuid := range map[string]string{"llm_logs": storyLogUUID, "agent_model_calls": chatLogUUID} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE uuid=?`, uuid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("down migration %s count=%d err=%v", table, count, err)
		}
	}
}

func TestLLMLogPayloadMigrationPreservesLegacyRowsAndJSONConstraints(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "llm-log-payloads.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(23); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-10T00:00:00Z"
	projectUUID := "019fea00-0000-7000-8000-000000000001"
	providerUUID := "019fea00-0000-7000-8000-000000000002"
	taskUUID := "019fea00-0000-7000-8000-000000000003"
	logUUID := "019fea00-0000-7000-8000-000000000004"
	if _, err := db.Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?,'Payload migration',1,1,?,?)`, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := db.QueryRow(`SELECT id FROM projects WHERE uuid=?`, projectUUID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO task_runs(uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,provider_uuid,model,progress,attempt,max_attempts,created_at,updated_at) VALUES(?,?,'story_chapter_generation',?,1,'{}','completed','payload-migration',?,'model',100,1,3,?,?)`, taskUUID, projectID, projectUUID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var taskID int64
	if err := db.QueryRow(`SELECT id FROM task_runs WHERE uuid=?`, taskUUID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO llm_logs(uuid,project_id,task_run_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_summary,output_summary,created_at,completed_at) VALUES(?,?,?,'story_generation','story_chapter_generation','text',1,?,'openai_compatible','model','completed','legacy input','legacy output',?,?)`, logUUID, projectID, taskID, providerUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var requestPayload, response sql.NullString
	if err := db.QueryRow(`SELECT request_payload,response FROM llm_logs WHERE uuid=?`, logUUID).Scan(&requestPayload, &response); err != nil || requestPayload.Valid || response.Valid {
		t.Fatalf("legacy payloads request=%+v response=%+v err=%v", requestPayload, response, err)
	}
	if _, err := db.Exec(`UPDATE llm_logs SET request_payload='not-json' WHERE uuid=?`, logUUID); err == nil {
		t.Fatal("invalid request payload JSON was accepted")
	}
	if _, err := db.Exec(`UPDATE llm_logs SET request_payload='{"model":"safe"}',response='{"content":"done"}' WHERE uuid=?`, logUUID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(23); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM llm_logs WHERE uuid=?`, logUUID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy row after rollback count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT request_payload FROM llm_logs WHERE uuid=?`, logUUID).Scan(&requestPayload); err == nil {
		t.Fatal("request_payload column survived rollback")
	}
}

func TestExpandedPromptEditingMigrationCanonicalizesLegacyHistoryAndRoundTripsRuntime(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "expanded-prompt-editing.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(21); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-10T00:00:00Z"
	projectUUID := "019fea90-0000-7000-8000-000000000001"
	actorUUID := "019fea90-0000-7000-8000-000000000002"
	legacyUUID := "019fea90-0000-7000-8000-000000000003"
	canonicalUUID := "019fea90-0000-7000-8000-000000000004"
	if _, err := db.Exec(`INSERT INTO projects(uuid,name,format_version,schema_version,created_at,updated_at) VALUES(?,'Prompts',1,1,?,?)`, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO actors(uuid,name,kind,created_at,updated_at) VALUES(?,'Owner','local_user',?,?)`, actorUUID, now, now); err != nil {
		t.Fatal(err)
	}
	var projectID, actorID int64
	if err := db.QueryRow(`SELECT id FROM projects WHERE uuid=?`, projectUUID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM actors WHERE uuid=?`, actorUUID).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_prompt_versions(uuid,project_id,actor_id,prompt_group,prompt_key,version_no,prompt,prompt_hash,source_type,created_at) VALUES(?,?,?,'premise','setting_generation',1,'Legacy','0000000000000000000000000000000000000000000000000000000000000000','manual_edit',?)`, legacyUUID, projectID, actorID, now); err != nil {
		t.Fatal(err)
	}
	var legacyID int64
	if err := db.QueryRow(`SELECT id FROM project_prompt_versions WHERE uuid=?`, legacyUUID).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_prompt_versions(uuid,project_id,actor_id,restored_from_version_id,prompt_group,prompt_key,version_no,prompt,prompt_hash,source_type,created_at) VALUES(?,?,?,?,'premise','setting_image',1,'Canonical','1111111111111111111111111111111111111111111111111111111111111111','version_restore',?)`, canonicalUUID, projectID, actorID, legacyID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var count, currentVersion int
	var currentPrompt string
	if err := db.QueryRow(`SELECT COUNT(*),MAX(version_no) FROM project_prompt_versions WHERE project_id=? AND prompt_group='premise' AND prompt_key='setting_image'`, projectID).Scan(&count, &currentVersion); err != nil || count != 2 || currentVersion != 2 {
		t.Fatalf("canonicalized history count=%d current=%d err=%v", count, currentVersion, err)
	}
	if err := db.QueryRow(`SELECT prompt FROM project_prompt_versions WHERE project_id=? AND prompt_group='premise' AND prompt_key='setting_image' ORDER BY version_no DESC LIMIT 1`, projectID).Scan(&currentPrompt); err != nil || currentPrompt != "Canonical" {
		t.Fatalf("canonical current=%q err=%v", currentPrompt, err)
	}
	var restoredFrom int64
	if err := db.QueryRow(`SELECT restored_from_version_id FROM project_prompt_versions WHERE uuid=?`, canonicalUUID).Scan(&restoredFrom); err != nil || restoredFrom != legacyID {
		t.Fatalf("restore relation=%d err=%v", restoredFrom, err)
	}
	runtimeUUID := "019fea90-0000-7000-8000-000000000005"
	if _, err := db.Exec(`INSERT INTO project_prompt_versions(uuid,project_id,actor_id,prompt_group,prompt_key,version_no,prompt,prompt_hash,source_type,created_at) VALUES(?,?,?,'runtime','project_language_instruction',1,'Language','2222222222222222222222222222222222222222222222222222222222222222','project_created',?)`, runtimeUUID, projectID, actorID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(21); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var group, key, source string
	if err := db.QueryRow(`SELECT prompt_group,prompt_key,source_type FROM project_prompt_versions WHERE uuid=?`, runtimeUUID).Scan(&group, &key, &source); err != nil || group != "agent" || key != "__lumi_runtime__.project_language_instruction" || source != "manual_edit" {
		t.Fatalf("runtime down identity=%s/%s source=%s err=%v", group, key, source, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT prompt_group,prompt_key FROM project_prompt_versions WHERE uuid=?`, runtimeUUID).Scan(&group, &key); err != nil || group != "runtime" || key != "project_language_instruction" {
		t.Fatalf("runtime re-up identity=%s/%s err=%v", group, key, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("expanded prompt migration left a foreign key violation")
	}
}

func TestLLMUsageMetricsMigrationUpAndDown(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "llm-usage-metrics.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"cached_input_tokens", "input_characters", "output_characters"} {
		if !tableHasColumn(t, db, "llm_logs", column) {
			t.Fatalf("llm_logs missing %s", column)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(17); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, column := range []string{"cached_input_tokens", "input_characters", "output_characters"} {
		if tableHasColumn(t, db, "llm_logs", column) {
			t.Fatalf("llm_logs retained %s after rollback", column)
		}
	}
}

func TestStoryChapterWorkflowMigrationPreservesExistingGraphAndDropsOnlyProjection(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "story-chapter-workflow.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	const (
		projectUUID     = "01900000-0000-7000-8000-000000000201"
		providerUUID    = "01900000-0000-7000-8000-000000000202"
		yoloThread      = "01900000-0000-7000-8000-000000000203"
		yoloWorkflow    = "01900000-0000-7000-8000-000000000204"
		yoloStep        = "01900000-0000-7000-8000-000000000205"
		chapterThread   = "01900000-0000-7000-8000-000000000206"
		chapterWorkflow = "01900000-0000-7000-8000-000000000207"
		chapterStep     = "01900000-0000-7000-8000-000000000208"
		batchThread     = "01900000-0000-7000-8000-000000000209"
		batchWorkflow   = "01900000-0000-7000-8000-000000000210"
		batchStep       = "01900000-0000-7000-8000-000000000211"
		taskUUID        = "01900000-0000-7000-8000-000000000212"
		auditThread     = "01900000-0000-7000-8000-000000000213"
		auditRun        = "01900000-0000-7000-8000-000000000214"
		eventUUID       = "01900000-0000-7000-8000-000000000215"
		logUUID         = "01900000-0000-7000-8000-000000000216"
		resourceUUID    = "01900000-0000-7000-8000-000000000217"
		now             = "2026-08-14T00:00:00Z"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','Story workflow migration',1,21,'` + now + `','` + now + `')`,
		`INSERT INTO task_runs(id,uuid,project_id,kind,resource_uuid,input_version,input_snapshot,status,idempotency_key,retryable,provider_uuid,model,model_source,progress,attempt,max_attempts,created_at,updated_at) VALUES(1,'` + taskUUID + `',1,'story_chapter_generation','` + resourceUUID + `',2,'{}','completed','story-migration-task',1,'` + providerUUID + `','model','provider_default',100,1,3,'` + now + `','` + now + `')`,
		`INSERT INTO agent_threads(id,uuid,project_id,kind,subject_type,subject_uuid,status,provider_uuid,model,created_at,updated_at) VALUES(1,'` + auditThread + `',1,'story_generation','chapter','` + resourceUUID + `','completed','` + providerUUID + `','model','` + now + `','` + now + `')`,
		`INSERT INTO agent_runs(id,uuid,agent_thread_id,task_run_id,trigger_type,status,created_at,updated_at) VALUES(1,'` + auditRun + `',1,1,'job_step','completed','` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(1,'` + yoloThread + `',1,'Yolo','completed','` + providerUUID + `','model','provider_default',1,1,1,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(2,'` + chapterThread + `',1,'Chapter','completed','` + providerUUID + `','model','provider_default',1,1,1,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(3,'` + batchThread + `',1,'Batch','completed','` + providerUUID + `','model','provider_default',1,1,1,'` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(1,'` + yoloWorkflow + `',1,1,'yolo_project_initialization','Yolo','completed',1,'{}','story-migration-yolo','` + providerUUID + `','model','provider_default','project_initialization','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(2,'` + chapterWorkflow + `',1,2,'story_chapter_generation','Chapter','completed',1,'{}','story-migration-chapter','` + providerUUID + `','model','provider_default','story_chapter','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(3,'` + batchWorkflow + `',1,3,'story_chapter_batch_plan','Batch','completed',1,'{}','story-migration-batch','` + providerUUID + `','model','provider_default','chapter_batch_plan','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(1,'` + yoloStep + `',1,'project_initialization',1,'completed','story-migration-yolo-step','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(2,'` + chapterStep + `',2,'story_chapter',1,'completed','story-migration-chapter-step','` + taskUUID + `','` + resourceUUID + `','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(3,'` + batchStep + `',3,'chapter_batch_plan',1,'completed','story-migration-batch-step','` + projectUUID + `','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_events(id,uuid,workflow_id,step_id,sequence,event_type,payload_json,created_at) VALUES(1,'` + eventUUID + `',1,1,1,'workflow_completed','{}','` + now + `')`,
		`INSERT INTO llm_logs(id,uuid,project_id,workflow_id,workflow_step_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_tokens,output_tokens,duration_ms,created_at,completed_at,request_payload,response,cached_input_tokens,input_characters,output_characters) VALUES(1,'` + logUUID + `',1,1,1,'workflow','migration_fixture','text',1,'` + providerUUID + `','openai_compatible','model','completed',10,5,20,'` + now + `','` + now + `','{}','{}',3,100,50)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(14); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var existingGraph, visibleProjections, rawTask, auditGraph int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM workflows WHERE uuid=?) + (SELECT COUNT(*) FROM workflow_steps WHERE uuid=?) + (SELECT COUNT(*) FROM workflow_events WHERE uuid=?) + (SELECT COUNT(*) FROM llm_logs WHERE uuid=?)`, yoloWorkflow, yoloStep, eventUUID, logUUID).Scan(&existingGraph); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM workflows WHERE uuid IN (?,?)) + (SELECT COUNT(*) FROM chat_threads WHERE uuid IN (?,?))`, chapterWorkflow, batchWorkflow, chapterThread, batchThread).Scan(&visibleProjections); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_runs WHERE uuid=?`, taskUUID).Scan(&rawTask); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM agent_threads WHERE uuid=?) + (SELECT COUNT(*) FROM agent_runs WHERE uuid=?)`, auditThread, auditRun).Scan(&auditGraph); err != nil {
		t.Fatal(err)
	}
	if existingGraph != 4 || visibleProjections != 0 || rawTask != 1 || auditGraph != 2 {
		t.Fatalf("down migration existing=%d visible=%d task=%d audit=%d", existingGraph, visibleProjections, rawTask, auditGraph)
	}
	for _, kind := range []string{"story_chapter_generation", "story_chapter_batch_plan"} {
		_, constraintErr := db.Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES('01900000-0000-7000-8000-000000000218',1,?,'Rejected','queued',1,'{}','rejected-story-kind','`+providerUUID+`','model','provider_default','story_chapter','`+now+`','`+now+`')`, kind)
		if constraintErr == nil {
			t.Fatalf("down migration still accepted %s", kind)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
}

func TestComicStoryboardWorkflowMigrationPreservesGraphAndRollsBackKind(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-storyboard-workflow.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	const (
		projectUUID       = "01900000-0000-7000-8000-000000000101"
		providerUUID      = "01900000-0000-7000-8000-000000000102"
		yoloThreadUUID    = "01900000-0000-7000-8000-000000000103"
		yoloWorkflowUUID  = "01900000-0000-7000-8000-000000000104"
		yoloStepUUID      = "01900000-0000-7000-8000-000000000105"
		storyThreadUUID   = "01900000-0000-7000-8000-000000000106"
		storyWorkflowUUID = "01900000-0000-7000-8000-000000000107"
		storyStepUUID     = "01900000-0000-7000-8000-000000000108"
		logUUID           = "01900000-0000-7000-8000-000000000109"
		now               = "2026-08-12T00:00:00Z"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','Storyboard migration',1,20,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(1,'` + yoloThreadUUID + `',1,'Yolo','completed','` + providerUUID + `','model','provider_default',1,1,1,'` + now + `','` + now + `')`,
		`INSERT INTO chat_threads(id,uuid,project_id,title,status,provider_uuid,model,model_source,next_turn_sequence,next_item_sequence,next_event_sequence,created_at,updated_at) VALUES(2,'` + storyThreadUUID + `',1,'Storyboard','completed','` + providerUUID + `','model','provider_default',1,1,1,'` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(1,'` + yoloWorkflowUUID + `',1,1,'yolo_project_initialization','Yolo','completed',1,'{}','migration-yolo','` + providerUUID + `','model','provider_default','project_initialization','` + now + `','` + now + `')`,
		`INSERT INTO workflows(id,uuid,project_id,thread_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES(2,'` + storyWorkflowUUID + `',1,2,'comic_storyboard_generation','Storyboard','completed',1,'{}','migration-storyboard','` + providerUUID + `','model','provider_default','comic_storyboard','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,input_json,output_json,created_at,updated_at) VALUES(1,'` + yoloStepUUID + `',1,'project_initialization',1,'completed','migration-yolo-step','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO workflow_steps(id,uuid,workflow_id,step_key,position,status,idempotency_key,task_uuid,resource_uuid,input_json,output_json,created_at,updated_at) VALUES(2,'` + storyStepUUID + `',2,'comic_storyboard',1,'completed','migration-storyboard-step','01900000-0000-7000-8000-000000000111','01900000-0000-7000-8000-000000000110','{}','{}','` + now + `','` + now + `')`,
		`INSERT INTO llm_logs(id,uuid,project_id,workflow_id,workflow_step_id,source_type,scenario,request_type,attempt,provider_uuid,provider_type,model,status,input_tokens,output_tokens,duration_ms,created_at,completed_at,request_payload,response,cached_input_tokens,input_characters,output_characters) VALUES(1,'` + logUUID + `',1,1,1,'workflow','migration_fixture','text',1,'` + providerUUID + `','openai_compatible','model','completed',10,5,20,'` + now + `','` + now + `','{}','{}',3,100,50)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(15); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var yoloCount, storyboardCount, storyboardThreadCount int
	if err := db.QueryRow(`SELECT count(*) FROM workflows WHERE uuid=?`, yoloWorkflowUUID).Scan(&yoloCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM workflows WHERE uuid=?`, storyWorkflowUUID).Scan(&storyboardCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM chat_threads WHERE uuid=?`, storyThreadUUID).Scan(&storyboardThreadCount); err != nil {
		t.Fatal(err)
	}
	var cached, inputCharacters, outputCharacters int
	if err := db.QueryRow(`SELECT cached_input_tokens,input_characters,output_characters FROM llm_logs WHERE uuid=?`, logUUID).Scan(&cached, &inputCharacters, &outputCharacters); err != nil {
		t.Fatal(err)
	}
	if yoloCount != 1 || storyboardCount != 0 || storyboardThreadCount != 0 || cached != 3 || inputCharacters != 100 || outputCharacters != 50 {
		t.Fatalf("down migration graph yolo=%d storyboard=%d thread=%d metrics=%d/%d/%d", yoloCount, storyboardCount, storyboardThreadCount, cached, inputCharacters, outputCharacters)
	}
	_, constraintErr := db.Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES('01900000-0000-7000-8000-000000000112',1,'comic_storyboard_generation','Rejected','queued',1,'{}','rejected-storyboard','` + providerUUID + `','model','provider_default','comic_storyboard','` + now + `','` + now + `')`)
	if constraintErr == nil {
		t.Fatal("down migration still accepted comic_storyboard_generation")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO workflows(uuid,project_id,kind,title,status,input_version,input_snapshot,idempotency_key,provider_uuid,model,model_source,current_step_key,created_at,updated_at) VALUES('01900000-0000-7000-8000-000000000113',1,'comic_storyboard_generation','Accepted','queued',1,'{}','accepted-storyboard','` + providerUUID + `','model','provider_default','comic_storyboard','` + now + `','` + now + `')`); err != nil {
		t.Fatalf("re-up rejected comic_storyboard_generation: %v", err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("comic storyboard workflow migration left a foreign key violation")
	}
}

func TestComicExportRetentionMigrationBackfillsAndRollsBack(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-export-retention.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(13); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	const (
		projectUUID = "01900000-0000-7000-8000-000000000301"
		objectUUID  = "01900000-0000-7000-8000-000000000302"
		fileUUID    = "01900000-0000-7000-8000-000000000303"
		readyUUID   = "01900000-0000-7000-8000-000000000304"
		failedUUID  = "01900000-0000-7000-8000-000000000305"
		sha         = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		snapshotSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	statements := []string{
		`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,'` + projectUUID + `','Retention migration',1,21,'2026-07-01T00:00:00Z','2026-07-01T00:00:00Z')`,
		`INSERT INTO file_objects(id,uuid,project_id,sha256,key_path,mime_type,canonical_ext,byte_size,state,created_at) VALUES(1,'` + objectUUID + `',1,'` + sha + `','aa/` + sha + `.zip','application/zip','zip',321,'ready','2026-08-01T00:00:00Z')`,
		`INSERT INTO files(id,uuid,project_id,file_object_id,kind,purpose,original_filename,source_type,metadata_json,created_at) VALUES(1,'` + fileUUID + `',1,1,'archive','export','legacy.zip','exported','{}','2026-08-01T00:00:00Z')`,
		`INSERT INTO comic_exports(id,uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,output_file_id,relative_path,created_at,completed_at) VALUES(1,'` + readyUUID + `',1,'01900000-0000-7000-8000-000000000306','project','zip','ready','{}','` + snapshotSHA + `',1,'exports/comic-project-bbbbbbbbbbbb.zip','2026-07-31T00:00:00Z','2026-08-01T00:00:00Z')`,
		`INSERT INTO comic_exports(id,uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,error_code,created_at) VALUES(2,'` + failedUUID + `',1,'01900000-0000-7000-8000-000000000307','project','zip','failed','{}','cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','','failed','2026-07-01T00:00:00Z')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var retentionDays int
	var readyExpiry, contentSHA string
	var byteSize int64
	if err := db.QueryRow(`SELECT retention_days,expires_at,byte_size,content_sha256 FROM comic_exports WHERE uuid=?`, readyUUID).Scan(&retentionDays, &readyExpiry, &byteSize, &contentSHA); err != nil {
		t.Fatal(err)
	}
	if retentionDays != 7 || readyExpiry != "2026-08-08T00:00:00Z" || byteSize != 321 || contentSHA != sha {
		t.Fatalf("ready backfill retention=%d expires=%q size=%d sha=%q", retentionDays, readyExpiry, byteSize, contentSHA)
	}
	var failedExpiry string
	if err := db.QueryRow(`SELECT expires_at FROM comic_exports WHERE uuid=?`, failedUUID).Scan(&failedExpiry); err != nil {
		t.Fatal(err)
	}
	if failedExpiry != "2026-07-08T00:00:00Z" {
		t.Fatalf("failed fallback expiry=%q", failedExpiry)
	}
	var expiryIndex int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='comic_exports_project_status_expiry_index'`).Scan(&expiryIndex); err != nil || expiryIndex != 1 {
		t.Fatalf("expiry index=%d err=%v", expiryIndex, err)
	}
	var legacyFileIndex int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='comic_exports_legacy_output_file_index'`).Scan(&legacyFileIndex); err != nil || legacyFileIndex != 1 {
		t.Fatalf("legacy output file index=%d err=%v", legacyFileIndex, err)
	}
	if _, err := db.Exec(`UPDATE comic_exports SET status='expired' WHERE uuid=?`, readyUUID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(13); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, column := range []string{"retention_days", "expires_at", "byte_size", "content_sha256"} {
		if tableHasColumn(t, db, "comic_exports", column) {
			t.Fatalf("down migration retained %s", column)
		}
	}
	var readyCount, failedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comic_exports WHERE uuid=?`, readyUUID).Scan(&readyCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comic_exports WHERE uuid=?`, failedUUID).Scan(&failedCount); err != nil {
		t.Fatal(err)
	}
	if readyCount != 0 || failedCount != 1 {
		t.Fatalf("down migration ready=%d failed=%d", readyCount, failedCount)
	}
}

func TestComicPDFExportMigrationPreservesZIPAndRollsBackPDFRows(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "comic-pdf-export.sqlite") + "?_pragma=foreign_keys(1)"
	runner, err := OpenProject(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(12); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	const (
		projectUUID = "01900000-0000-7000-8000-000000000401"
		zipUUID     = "01900000-0000-7000-8000-000000000402"
		pdfUUID     = "01900000-0000-7000-8000-000000000403"
		zipTaskUUID = "01900000-0000-7000-8000-000000000404"
		pdfTaskUUID = "01900000-0000-7000-8000-000000000405"
		zipHash     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		pdfHash     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		now         = "2026-08-15T00:00:00Z"
	)
	if _, err := db.Exec(`INSERT INTO projects(id,uuid,name,format_version,schema_version,created_at,updated_at) VALUES(1,?,'PDF migration',1,23,?,?)`, projectUUID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,byte_size,content_sha256,created_at) VALUES(?,1,?,'project','zip','queued','{}',?,'exports/existing.zip',7,0,'',?)`, zipUUID, zipTaskUUID, zipHash, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,byte_size,content_sha256,created_at) VALUES(?,1,?,'project','pdf','queued','{}',?,'exports/rejected.pdf',7,0,'',?)`, pdfUUID, pdfTaskUUID, pdfHash, now); err == nil {
		t.Fatal("pre-migration schema accepted pdf")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var zipCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comic_exports WHERE uuid=? AND format='zip'`, zipUUID).Scan(&zipCount); err != nil || zipCount != 1 {
		t.Fatalf("zip count=%d err=%v", zipCount, err)
	}
	if _, err := db.Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,byte_size,content_sha256,created_at) VALUES(?,1,?,'project','pdf','queued','{}',?,'exports/accepted.pdf',7,0,'',?)`, pdfUUID, pdfTaskUUID, pdfHash, now); err != nil {
		t.Fatalf("pdf insert: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runner.Down(12); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pdfCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comic_exports WHERE uuid=?`, pdfUUID).Scan(&pdfCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM comic_exports WHERE uuid=?`, zipUUID).Scan(&zipCount); err != nil {
		t.Fatal(err)
	}
	if pdfCount != 0 || zipCount != 1 {
		t.Fatalf("down migration pdf=%d zip=%d", pdfCount, zipCount)
	}
	if _, err := db.Exec(`INSERT INTO comic_exports(uuid,project_id,task_uuid,scope,format,status,snapshot_json,snapshot_hash,relative_path,retention_days,byte_size,content_sha256,created_at) VALUES(?,1,?,'project','pdf','queued','{}',?,'exports/rejected-again.pdf',7,0,'',?)`, pdfUUID, pdfTaskUUID, pdfHash, now); err == nil {
		t.Fatal("down migration still accepted pdf")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("comic PDF export migration left a foreign key violation")
	}
}

func TestRunnerUpVersionAndDown(t *testing.T) {
	migrationFS := fstest.MapFS{
		"000001_create_example.up.sql":   {Data: []byte("CREATE TABLE examples (id INTEGER PRIMARY KEY AUTOINCREMENT);")},
		"000001_create_example.down.sql": {Data: []byte("DROP TABLE examples;")},
	}
	runner, err := OpenWithFS("file:"+t.TempDir()+"/migrations.sqlite3?_pragma=foreign_keys(1)", migrationFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })

	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	version, dirty, applied, err := runner.Version()
	if err != nil || version != 1 || dirty || !applied {
		t.Fatalf("Version() = %d, %v, %v, %v", version, dirty, applied, err)
	}
	if err := runner.Down(1); err != nil {
		t.Fatal(err)
	}
	_, _, applied, err = runner.Version()
	if err != nil || applied {
		t.Fatalf("empty Version() = applied %v, error %v", applied, err)
	}
}

func TestRunnerAcceptsEmptyMigrationSet(t *testing.T) {
	dsn := "file:" + t.TempDir() + "/empty.sqlite3"
	runner, err := OpenWithFS(dsn, fstest.MapFS{"README.md": {Data: []byte("empty")}}, ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	if err := runner.Up(); !IsNoChange(err) {
		t.Fatalf("Up() error = %v, want ErrNoChange", err)
	}
	if err := runner.Down(1); !IsNoChange(err) {
		t.Fatalf("Down() error = %v, want ErrNoChange", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0] != "schema_migrations" {
		t.Fatalf("tables = %v, want only schema_migrations", tables)
	}
}

func TestRunnerReportsDirtyMigration(t *testing.T) {
	migrationFS := fstest.MapFS{
		"000001_broken.up.sql":   {Data: []byte("THIS IS NOT SQL;")},
		"000001_broken.down.sql": {Data: []byte("SELECT 1;")},
	}
	runner, err := OpenWithFS("file:"+t.TempDir()+"/dirty.sqlite3", migrationFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runner.Close() })

	if err := runner.Up(); err == nil {
		t.Fatal("Up() succeeded for invalid SQL")
	}
	version, dirty, applied, err := runner.Version()
	if err != nil || version != 1 || !dirty || !applied {
		t.Fatalf("Version() = %d, %v, %v, %v", version, dirty, applied, err)
	}
}

func tableNames(t *testing.T, dsn string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

func containsTable(tables []string, name string) bool {
	for _, table := range tables {
		if table == name {
			return true
		}
	}
	return false
}

func tableHasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 1
}
