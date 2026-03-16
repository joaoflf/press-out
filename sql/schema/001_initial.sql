CREATE TABLE IF NOT EXISTS lifts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lift_type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    coaching_cue TEXT,
    coaching_diagnosis TEXT
);
