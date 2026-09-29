create or replace function guard_booking_skill_mapping_write()
returns trigger
language plpgsql
as $$
begin
  perform pg_advisory_xact_lock(810245902);
  return null;
end;
$$;

create trigger service_requirement_write_guard
before insert or update or delete on service_type_required_skills
for each statement execute function guard_booking_skill_mapping_write();

create trigger technician_skill_write_guard
before insert or update or delete on technician_skills
for each statement execute function guard_booking_skill_mapping_write();

create or replace function confirm_appointment(
  p_vehicle_id uuid,
  p_dealership_id uuid,
  p_service_type_id uuid,
  p_start_at timestamptz
) returns appointments
language plpgsql
as $$
declare
  selected_customer_id uuid;
  selected_duration_minutes integer;
  selected_timezone text;
  selected_end_at timestamptz;
  local_start timestamp;
  local_end timestamp;
  candidate record;
  saved_constraint text;
  confirmed appointments%rowtype;
begin
  if p_start_at is null then
    raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'INVALID_REFERENCE';
  end if;

  select st.duration_minutes
    into selected_duration_minutes
  from service_types st
  where st.id = p_service_type_id and st.active
  for share of st;

  if not found then
    raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'INVALID_REFERENCE';
  end if;

  perform skill.id
  from skills skill
  order by skill.id
  for share of skill;

  select v.customer_id, d.timezone
    into selected_customer_id, selected_timezone
  from vehicles v
  join customers c on c.id = v.customer_id and c.active
  join dealerships d on d.id = p_dealership_id and d.active
  where v.id = p_vehicle_id and v.active
  for share of v, c, d;

  if not found then
    raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'INVALID_REFERENCE';
  end if;

  perform t.id
  from technicians t
  where t.dealership_id = p_dealership_id
  order by t.id
  for share of t;

  perform b.id
  from service_bays b
  where b.dealership_id = p_dealership_id
  order by b.id
  for share of b;

  -- Parent rows are locked before this shared configuration lock. Mapping
  -- DML takes the exclusive form in a BEFORE STATEMENT trigger, so direct SQL
  -- writes and row-then-mapping admin transactions cannot invert lock order.
  perform pg_advisory_xact_lock_shared(810245902);

  if not exists (
      select 1 from service_type_required_skills required
      join skills skill on skill.id = required.skill_id and skill.active
      where required.service_type_id = p_service_type_id
    ) or exists (
      select 1 from service_type_required_skills required
      left join skills skill on skill.id = required.skill_id and skill.active
      where required.service_type_id = p_service_type_id and skill.id is null
    )
  then
    raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'INVALID_REFERENCE';
  end if;

  selected_end_at := p_start_at + make_interval(mins => selected_duration_minutes);
  local_start := p_start_at at time zone selected_timezone;
  local_end := selected_end_at at time zone selected_timezone;

  if extract(second from local_start) <> 0
    or mod((extract(hour from local_start)::integer * 60) + extract(minute from local_start)::integer, 30) <> 0
    or not exists (
      select 1
      from dealership_business_hours hours
      where hours.dealership_id = p_dealership_id
        and hours.day_of_week = extract(dow from local_start)::integer
        and local_start::date = local_end::date
        and local_start::time >= hours.opens_at
        and local_end::time <= hours.closes_at
    )
  then
    raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'INVALID_REQUEST';
  end if;

  for candidate in
    with eligible_technicians as (
      select t.id,
        count(a.id) filter (where a.start_at >= p_start_at) as future_load
      from technicians t
      left join appointments a on a.technician_id = t.id and a.status = 'CONFIRMED'
      where t.dealership_id = p_dealership_id and t.active
        and not exists (
          select 1
          from service_type_required_skills required
          where required.service_type_id = p_service_type_id
            and not exists (
              select 1 from technician_skills possessed
              join skills skill on skill.id = possessed.skill_id and skill.active
              where possessed.technician_id = t.id
                and possessed.skill_id = required.skill_id
            )
        )
        and not exists (
          select 1 from appointments busy
          where busy.technician_id = t.id and busy.status = 'CONFIRMED'
            and tstzrange(busy.start_at, busy.end_at, '[)') && tstzrange(p_start_at, selected_end_at, '[)')
        )
      group by t.id
    ), eligible_bays as (
      select b.id,
        count(a.id) filter (where a.start_at >= p_start_at) as future_load
      from service_bays b
      left join appointments a on a.service_bay_id = b.id and a.status = 'CONFIRMED'
      where b.dealership_id = p_dealership_id and b.active
        and not exists (
          select 1 from appointments busy
          where busy.service_bay_id = b.id and busy.status = 'CONFIRMED'
            and tstzrange(busy.start_at, busy.end_at, '[)') && tstzrange(p_start_at, selected_end_at, '[)')
        )
      group by b.id
    )
    select t.id as technician_id, b.id as bay_id
    from eligible_technicians t cross join eligible_bays b
    order by t.future_load, t.id, b.future_load, b.id
  loop
    begin
      insert into appointments (
        customer_id, vehicle_id, dealership_id, service_type_id,
        technician_id, service_bay_id, status, start_at, end_at
      ) values (
        selected_customer_id, p_vehicle_id, p_dealership_id, p_service_type_id,
        candidate.technician_id, candidate.bay_id, 'CONFIRMED', p_start_at, selected_end_at
      ) returning * into confirmed;
      return confirmed;
    exception when exclusion_violation then
      get stacked diagnostics saved_constraint = constraint_name;
      if saved_constraint not in ('appointments_technician_no_overlap', 'appointments_bay_no_overlap') then
        raise;
      end if;
    end;
  end loop;

  raise exception using errcode = 'P0001', message = 'booking rejected', detail = 'RESOURCE_CONFLICT';
end;
$$;
