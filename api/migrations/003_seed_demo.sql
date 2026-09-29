insert into customers (id, name, email, active)
values
  ('20000000-0000-0000-0000-000000000001', 'Jordan Lee', 'jordan.lee@example.invalid', true),
  ('20000000-0000-0000-0000-000000000002', 'Archived Customer', 'archived.customer@example.invalid', false)
on conflict (id) do update set
  name = excluded.name,
  email = excluded.email,
  active = excluded.active;

insert into dealerships (id, name, address, timezone, active)
values
  ('20000000-0000-0000-0000-000000000021', 'Riverside Service Centre', '100 Riverside Way', 'Europe/London', true),
  ('20000000-0000-0000-0000-000000000022', 'Archived Service Centre', '200 Archive Road', 'Europe/London', false)
on conflict (id) do update set
  name = excluded.name,
  address = excluded.address,
  timezone = excluded.timezone,
  active = excluded.active;

insert into dealership_business_hours (dealership_id, day_of_week, opens_at, closes_at)
values
  ('20000000-0000-0000-0000-000000000021', 0, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 1, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 2, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 3, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 4, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 5, '09:00', '17:00'),
  ('20000000-0000-0000-0000-000000000021', 6, '09:00', '17:00')
on conflict (dealership_id, day_of_week) do update set
  opens_at = excluded.opens_at,
  closes_at = excluded.closes_at;

insert into vehicles (id, customer_id, label, registration, active)
values
  ('20000000-0000-0000-0000-000000000011', '20000000-0000-0000-0000-000000000001', 'Silver Hatchback', 'DEMO-001', true),
  ('20000000-0000-0000-0000-000000000012', '20000000-0000-0000-0000-000000000002', 'Archived Saloon', 'DEMO-002', false)
on conflict (id) do update set
  customer_id = excluded.customer_id,
  label = excluded.label,
  registration = excluded.registration,
  active = excluded.active;

insert into skills (id, name, active)
values
  ('20000000-0000-0000-0000-000000000031', 'Routine Maintenance', true),
  ('20000000-0000-0000-0000-000000000032', 'Diagnostic Inspection', true),
  ('20000000-0000-0000-0000-000000000033', 'Archived Skill', false)
on conflict (id) do update set
  name = excluded.name,
  active = excluded.active;

insert into service_types (id, name, description, duration_minutes, active)
values
  ('20000000-0000-0000-0000-000000000041', 'Routine Inspection', 'A standard safety and maintenance inspection.', 60, true),
  ('20000000-0000-0000-0000-000000000042', 'Comprehensive Service', 'A full maintenance and diagnostic service.', 90, true),
  ('20000000-0000-0000-0000-000000000043', 'Archived Service', 'An inactive service retained for filtering tests.', 30, false)
on conflict (id) do update set
  name = excluded.name,
  description = excluded.description,
  duration_minutes = excluded.duration_minutes,
  active = excluded.active;

insert into service_type_required_skills (service_type_id, skill_id)
values
  ('20000000-0000-0000-0000-000000000041', '20000000-0000-0000-0000-000000000031'),
  ('20000000-0000-0000-0000-000000000042', '20000000-0000-0000-0000-000000000031'),
  ('20000000-0000-0000-0000-000000000042', '20000000-0000-0000-0000-000000000032'),
  ('20000000-0000-0000-0000-000000000043', '20000000-0000-0000-0000-000000000033')
on conflict (service_type_id, skill_id) do nothing;

insert into technicians (id, dealership_id, name, active)
values
  ('20000000-0000-0000-0000-000000000051', '20000000-0000-0000-0000-000000000021', 'Taylor Morgan', true),
  ('20000000-0000-0000-0000-000000000052', '20000000-0000-0000-0000-000000000021', 'Avery Chen', true),
  ('20000000-0000-0000-0000-000000000053', '20000000-0000-0000-0000-000000000021', 'Robin Quinn', true),
  ('20000000-0000-0000-0000-000000000054', '20000000-0000-0000-0000-000000000021', 'Archived Technician', false)
on conflict (id) do update set
  dealership_id = excluded.dealership_id,
  name = excluded.name,
  active = excluded.active;

insert into technician_skills (technician_id, skill_id)
values
  ('20000000-0000-0000-0000-000000000051', '20000000-0000-0000-0000-000000000031'),
  ('20000000-0000-0000-0000-000000000051', '20000000-0000-0000-0000-000000000032'),
  ('20000000-0000-0000-0000-000000000052', '20000000-0000-0000-0000-000000000031'),
  ('20000000-0000-0000-0000-000000000054', '20000000-0000-0000-0000-000000000031')
on conflict (technician_id, skill_id) do nothing;

insert into service_bays (id, dealership_id, name, active)
values
  ('20000000-0000-0000-0000-000000000061', '20000000-0000-0000-0000-000000000021', 'Bay A', true),
  ('20000000-0000-0000-0000-000000000062', '20000000-0000-0000-0000-000000000021', 'Bay B', true),
  ('20000000-0000-0000-0000-000000000063', '20000000-0000-0000-0000-000000000021', 'Archived Bay', false)
on conflict (id) do update set
  dealership_id = excluded.dealership_id,
  name = excluded.name,
  active = excluded.active;

insert into appointments (
  id,
  customer_id,
  vehicle_id,
  dealership_id,
  service_type_id,
  technician_id,
  service_bay_id,
  status,
  start_at,
  end_at
)
values (
  '20000000-0000-0000-0000-000000000071',
  '20000000-0000-0000-0000-000000000001',
  '20000000-0000-0000-0000-000000000011',
  '20000000-0000-0000-0000-000000000021',
  '20000000-0000-0000-0000-000000000041',
  '20000000-0000-0000-0000-000000000051',
  '20000000-0000-0000-0000-000000000061',
  'CONFIRMED',
  '2030-01-02 10:00:00+00',
  '2030-01-02 11:00:00+00'
)
on conflict (id) do update set
  customer_id = excluded.customer_id,
  vehicle_id = excluded.vehicle_id,
  dealership_id = excluded.dealership_id,
  service_type_id = excluded.service_type_id,
  technician_id = excluded.technician_id,
  service_bay_id = excluded.service_bay_id,
  status = excluded.status,
  start_at = excluded.start_at,
  end_at = excluded.end_at;
