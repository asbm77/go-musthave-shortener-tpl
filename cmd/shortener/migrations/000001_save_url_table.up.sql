CREATE TABLE SAVE_URL_TABLE (
                        id SERIAL PRIMARY KEY,
                        shorturl VARCHAR(255) NOT NULL,
                        url TEXT NOT NULL

);

CREATE INDEX SAVE_URL_TABLE ON movies(shorturl);

