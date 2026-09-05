CREATE TABLE model_prices (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 uuid TEXT NOT NULL UNIQUE CHECK(length(uuid)=36),
 provider_uuid TEXT NOT NULL DEFAULT '',
 provider_type TEXT NOT NULL,
 model TEXT NOT NULL,
 region TEXT NOT NULL DEFAULT '',
 request_type TEXT NOT NULL CHECK(request_type IN ('text','image')),
 source TEXT NOT NULL CHECK(source IN ('builtin','user')),
 catalog_key TEXT UNIQUE,
 rule_json TEXT NOT NULL CHECK(json_valid(rule_json)),
 active INTEGER NOT NULL DEFAULT 1 CHECK(active IN (0,1)),
 created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX model_prices_active ON model_prices(source,provider_uuid,provider_type,model,region,request_type) WHERE active=1;
