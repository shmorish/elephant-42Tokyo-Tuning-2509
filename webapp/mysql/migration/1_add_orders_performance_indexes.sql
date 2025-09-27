-- Performance optimization indexes for orders table
-- These indexes will significantly improve /api/v1/orders endpoint performance

-- Primary index for user-based queries (most common query pattern)
CREATE INDEX idx_orders_user_id ON orders(user_id);

-- Composite index for user_id + created_at (for sorting by creation date)
CREATE INDEX idx_orders_user_created ON orders(user_id, created_at);

-- Composite index for user_id + shipped_status (for filtering by status)
CREATE INDEX idx_orders_user_status ON orders(user_id, shipped_status);

-- Index on product_id for JOIN operations with products table
CREATE INDEX idx_orders_product_id ON orders(product_id);

-- Index on arrived_at for sorting operations
CREATE INDEX idx_orders_arrived_at ON orders(arrived_at);

-- Index on products.name for search functionality
CREATE INDEX idx_products_name ON products(name);