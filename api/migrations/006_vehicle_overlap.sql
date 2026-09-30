alter table appointments
  add constraint appointments_vehicle_no_overlap
  exclude using gist (
    vehicle_id with =,
    tstzrange(start_at, end_at, '[)') with &&
  ) where (status = 'CONFIRMED');
