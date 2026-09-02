CREATE TABLE IF NOT EXISTS tastings (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  distillery TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL DEFAULT '',
  age INTEGER CHECK (age IS NULL OR age >= 0),
  abv NUMERIC(5,2) CHECK (abv IS NULL OR (abv >= 0 AND abv <= 100)),
  cask_type TEXT NOT NULL DEFAULT '',
  bottler TEXT NOT NULL DEFAULT '',
  rating INTEGER CHECK (rating IS NULL OR (rating >= 0 AND rating <= 100)),
  date_tasted DATE NOT NULL DEFAULT CURRENT_DATE,
  tags TEXT[] NOT NULL DEFAULT '{}',
  nose TEXT NOT NULL DEFAULT '',
  palate TEXT NOT NULL DEFAULT '',
  finish TEXT NOT NULL DEFAULT '',
  overall TEXT NOT NULL DEFAULT '',
  what_learned TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS tastings_date_tasted_idx ON tastings (date_tasted DESC);
