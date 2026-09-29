alter table appointments
  add constraint appointments_technician_no_overlap
  exclude using gist (
    technician_id with =,
    tstzrange(start_at, end_at, '[)') with &&
  ) where (status = 'CONFIRMED');

alter table appointments
  add constraint appointments_bay_no_overlap
  exclude using gist (
    service_bay_id with =,
    tstzrange(start_at, end_at, '[)') with &&
  ) where (status = 'CONFIRMED');

create index appointments_dealership_interval_idx
  on appointments using gist (
    dealership_id,
    tstzrange(start_at, end_at, '[)')
  ) where (status = 'CONFIRMED');

create index appointments_vehicle_id_idx on appointments (vehicle_id);
create index appointments_service_type_id_idx on appointments (service_type_id);

create index technicians_active_dealership_idx
  on technicians (dealership_id, id) where active;

create index service_bays_active_dealership_idx
  on service_bays (dealership_id, id) where active;

create index technician_skills_skill_technician_idx
  on technician_skills (skill_id, technician_id);

create index service_required_skills_skill_service_idx
  on service_type_required_skills (skill_id, service_type_id);
