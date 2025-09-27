-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
ALTER TABLE `products` ADD INDEX `idx_name_product_id` (`name`, `product_id`);
ALTER TABLE users ADD INDEX idx_users_user_name (user_name);
create or replace view pf as
select
  SCHEMA_NAME,
  -- DIGEST,
  DIGEST_TEXT,
  COUNT_STAR,
  round(SUM_TIMER_WAIT / pow(1000,4), 3) as SUM_TIMER_WAIT_SEC,
  round(MIN_TIMER_WAIT / pow(1000,4), 3) as MIN_TIMER_WAIT_SEC,
  round(AVG_TIMER_WAIT / pow(1000,4), 3) as AVG_TIMER_WAIT_SEC,
  round(MAX_TIMER_WAIT / pow(1000,4), 3) as MAX_TIMER_WAIT_SEC,
  round(SUM_LOCK_TIME / pow(1000,4), 3) as SUM_LOCK_TIME_SEC,
  round(QUANTILE_95 / pow(1000,4), 3) as P95,
  round(QUANTILE_99 / pow(1000,4), 3) as P99,
  round(QUANTILE_999 / pow(1000,4), 3) as P999,
  SUM_ERRORS,
  SUM_WARNINGS,
  SUM_ROWS_AFFECTED,
  SUM_ROWS_SENT,
  SUM_ROWS_EXAMINED,
  SUM_CREATED_TMP_DISK_TABLES,
  SUM_CREATED_TMP_TABLES,
  SUM_SELECT_FULL_JOIN,
  SUM_SELECT_FULL_RANGE_JOIN,
  SUM_SELECT_RANGE,
  SUM_SELECT_RANGE_CHECK,
  SUM_SELECT_SCAN,
  SUM_SORT_MERGE_PASSES,
  SUM_SORT_RANGE,
  SUM_SORT_ROWS,
  SUM_SORT_SCAN,
  SUM_NO_INDEX_USED,
  SUM_NO_GOOD_INDEX_USED,
  -- round(SUM_CPU_TIME / pow(1000,4), 3) as SUM_CPU_TIME_SEC,
  round(MAX_CONTROLLED_MEMORY / pow(1024,2), 3) as MAX_CONTROLLED_MEMORY_MB,
  round(MAX_TOTAL_MEMORY / pow(1024,2), 3) as MAX_TOTAL_MEMORY_MB,
  -- COUNT_SECONDARY,
  -- FIRST_SEEN,
  -- LAST_SEEN,
  QUERY_SAMPLE_TEXT,
  -- QUERY_SAMPLE_SEEN,
  round(QUERY_SAMPLE_TIMER_WAIT / pow(1000,4), 3) as QUERY_SAMPLE_TIMER_WAIT_SEC
from
  performance_schema.events_statements_summary_by_digest
where
  `SCHEMA_NAME` != 'performance_schema'
  AND `SCHEMA_NAME` IS NOT NULL
order by
  SUM_TIMER_WAIT desc
limit
  3\G;


-- Used in: session.go:42-43 (JOIN user_sessions s ON u.user_id = s.user_id WHERE s.session_uuid = ?)
CREATE INDEX idx_user_sessions_uuid_expires ON user_sessions(session_uuid, expires_at, user_id);

-- Used in: user.go:23 (WHERE user_name = ?)
CREATE INDEX idx_users_user_name ON users(user_name);

-- Used in: order.go:111 (WHERE o.shipped_status = 'shipping')
CREATE INDEX idx_orders_shipped_status ON orders(shipped_status, product_id);

-- Used in: product.go:115,133 (WHERE (name LIKE ? OR description LIKE ?))
CREATE INDEX idx_products_description ON products(description(100));

-- Used in: order.go:136-146 (ORDER BY various fields)
CREATE INDEX idx_orders_user_created_desc ON orders(user_id, created_at DESC);
CREATE INDEX idx_orders_user_arrived ON orders(user_id, arrived_at);

-- Used in: order.go:177-182 (SELECT order_id, product_id, shipped_status, created_at, arrived_at)
CREATE INDEX idx_orders_covering ON orders(user_id, order_id, product_id, shipped_status, created_at, arrived_at);
