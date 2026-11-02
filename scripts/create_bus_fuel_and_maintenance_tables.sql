-- Migration: Create bus_fuel_logs and bus_maintenance_logs tables
-- Target Database: PostgreSQL (Supabase)

-- 1. Create bus_fuel_logs table
CREATE TABLE IF NOT EXISTS bus_fuel_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bus_id UUID NOT NULL REFERENCES buses(id) ON DELETE CASCADE,
    staff_id UUID NOT NULL REFERENCES bus_staff(id) ON DELETE CASCADE,
    bus_owner_id UUID NOT NULL REFERENCES bus_owners(id) ON DELETE CASCADE,
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    fuel_type VARCHAR(20) NOT NULL DEFAULT 'diesel',
    liters DECIMAL(10,2) NOT NULL CHECK (liters > 0),
    cost_per_liter DECIMAL(10,2) NULL,
    total_cost DECIMAL(12,2) NOT NULL CHECK (total_cost >= 0),
    odometer_reading INTEGER NULL,
    station_name VARCHAR(255) NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for bus_fuel_logs
CREATE INDEX IF NOT EXISTS idx_bus_fuel_logs_bus_id ON bus_fuel_logs(bus_id);
CREATE INDEX IF NOT EXISTS idx_bus_fuel_logs_staff_id ON bus_fuel_logs(staff_id);
CREATE INDEX IF NOT EXISTS idx_bus_fuel_logs_bus_owner_id ON bus_fuel_logs(bus_owner_id);
CREATE INDEX IF NOT EXISTS idx_bus_fuel_logs_date ON bus_fuel_logs(date DESC);

-- 2. Create bus_maintenance_logs table
CREATE TABLE IF NOT EXISTS bus_maintenance_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bus_id UUID NOT NULL REFERENCES buses(id) ON DELETE CASCADE,
    staff_id UUID NOT NULL REFERENCES bus_staff(id) ON DELETE CASCADE,
    bus_owner_id UUID NOT NULL REFERENCES bus_owners(id) ON DELETE CASCADE,
    date DATE NOT NULL DEFAULT CURRENT_DATE,
    maintenance_type VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    total_cost DECIMAL(12,2) NOT NULL CHECK (total_cost >= 0),
    vendor_name VARCHAR(255) NULL,
    odometer_reading INTEGER NULL,
    next_service_due_km INTEGER NULL,
    next_service_due_date DATE NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for bus_maintenance_logs
CREATE INDEX IF NOT EXISTS idx_bus_maintenance_logs_bus_id ON bus_maintenance_logs(bus_id);
CREATE INDEX IF NOT EXISTS idx_bus_maintenance_logs_staff_id ON bus_maintenance_logs(staff_id);
CREATE INDEX IF NOT EXISTS idx_bus_maintenance_logs_bus_owner_id ON bus_maintenance_logs(bus_owner_id);
CREATE INDEX IF NOT EXISTS idx_bus_maintenance_logs_date ON bus_maintenance_logs(date DESC);

