create extension if not exists btree_gist;

create table customers (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  email text not null unique,
  active boolean not null default true,
  created_at timestamptz not null default now()
);

create table dealerships (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  address text not null,
  timezone text not null,
  active boolean not null default true,
  created_at timestamptz not null default now()
);

create table dealership_business_hours (
  dealership_id uuid not null references dealerships(id) on delete cascade,
  day_of_week smallint not null,
  opens_at time not null,
  closes_at time not null,
  primary key (dealership_id, day_of_week),
  constraint dealership_business_hours_day_check
    check (day_of_week between 0 and 6),
  constraint dealership_business_hours_interval_check
    check (opens_at < closes_at)
);

create table vehicles (
  id uuid primary key default gen_random_uuid(),
  customer_id uuid not null references customers(id),
  label text not null,
  registration text not null unique,
  active boolean not null default true,
  created_at timestamptz not null default now(),
  unique (id, customer_id)
);

create table skills (
  id uuid primary key default gen_random_uuid(),
  name text not null unique,
  active boolean not null default true
);

create table service_types (
  id uuid primary key default gen_random_uuid(),
  name text not null unique,
  description text not null,
  duration_minutes integer not null,
  active boolean not null default true,
  constraint service_types_duration_check check (duration_minutes > 0)
);

create table service_type_required_skills (
  service_type_id uuid not null references service_types(id) on delete cascade,
  skill_id uuid not null references skills(id),
  primary key (service_type_id, skill_id)
);

create table technicians (
  id uuid primary key default gen_random_uuid(),
  dealership_id uuid not null references dealerships(id),
  name text not null,
  active boolean not null default true,
  unique (id, dealership_id)
);

create table technician_skills (
  technician_id uuid not null references technicians(id) on delete cascade,
  skill_id uuid not null references skills(id),
  primary key (technician_id, skill_id)
);

create table service_bays (
  id uuid primary key default gen_random_uuid(),
  dealership_id uuid not null references dealerships(id),
  name text not null,
  active boolean not null default true,
  unique (id, dealership_id),
  unique (dealership_id, name)
);

create table appointments (
  id uuid primary key default gen_random_uuid(),
  customer_id uuid not null references customers(id),
  vehicle_id uuid not null references vehicles(id),
  dealership_id uuid not null references dealerships(id),
  service_type_id uuid not null references service_types(id),
  technician_id uuid not null references technicians(id),
  service_bay_id uuid not null references service_bays(id),
  status text not null default 'CONFIRMED',
  start_at timestamptz not null,
  end_at timestamptz not null,
  created_at timestamptz not null default now(),
  constraint appointments_status_check
    check (status in ('CONFIRMED', 'CANCELLED')),
  constraint appointments_interval_check check (start_at < end_at),
  constraint appointments_vehicle_owner_fk
    foreign key (vehicle_id, customer_id) references vehicles(id, customer_id),
  constraint appointments_technician_dealership_fk
    foreign key (technician_id, dealership_id) references technicians(id, dealership_id),
  constraint appointments_bay_dealership_fk
    foreign key (service_bay_id, dealership_id) references service_bays(id, dealership_id)
);

create table idempotency_records (
  idempotency_key text primary key,
  request_hash text not null,
  appointment_id uuid references appointments(id),
  created_at timestamptz not null default now()
);
