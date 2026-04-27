CREATE TABLE SAVE_URL_TABLE (
                        id SERIAL PRIMARY KEY,
                        shorturl VARCHAR(255) NOT NULL,
                        url TEXT NOT NULL UNIQUE

);

CREATE INDEX SAVE_URL_TABLE ON SAVE_URL_TABLE(shorturl);

