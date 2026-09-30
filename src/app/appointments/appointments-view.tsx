import Link from "next/link";

import { createBookingApi } from "@/features/booking/api/booking-api";
import {
  BookingApiError,
  type ConfirmedAppointment,
} from "@/features/booking/api/contracts";

import styles from "./appointments.module.css";

type Status = ConfirmedAppointment["status"];

const filters: Array<{ label: string; status?: Status }> = [
  { label: "All" },
  { label: "Confirmed", status: "CONFIRMED" },
  { label: "Cancelled", status: "CANCELLED" },
];

function formatDate(value: string, timeZone: string) {
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
    timeZone,
  }).format(new Date(value));
}

function formatTime(value: string, timeZone: string) {
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone,
  }).format(new Date(value));
}

function AppointmentCard({
  appointment,
}: {
  appointment: ConfirmedAppointment;
}) {
  const { timezone } = appointment.dealership;
  return (
    <article className={styles.card}>
      <div className={styles.dateBlock}>
        <strong>{formatDate(appointment.startAt, timezone)}</strong>
        <span>
          {formatTime(appointment.startAt, timezone)}–
          {formatTime(appointment.endAt, timezone)}
        </span>
      </div>
      <div className={styles.primaryDetails}>
        <span
          className={`${styles.status} ${appointment.status === "CANCELLED" ? styles.cancelled : ""}`}
        >
          {appointment.status}
        </span>
        <h2>{appointment.vehicle.label}</h2>
        <p>{appointment.vehicle.registration}</p>
      </div>
      <div className={styles.detail}>
        <span>Service</span>
        <strong>{appointment.serviceType.name}</strong>
      </div>
      <div className={styles.detail}>
        <span>Service centre</span>
        <strong>{appointment.dealership.name}</strong>
      </div>
      <div className={styles.detail}>
        <span>Resources</span>
        <strong>
          {appointment.technician.name} · {appointment.serviceBay.name}
        </strong>
      </div>
      <Link
        className={styles.viewLink}
        href={`/appointments/${appointment.id}`}
        aria-label={`View appointment for ${appointment.vehicle.label}`}
      >
        View appointment <span aria-hidden="true">→</span>
      </Link>
    </article>
  );
}

export async function renderAppointmentsPage({
  status,
  apiBaseURL,
  fetchImpl = globalThis.fetch,
}: {
  status?: Status;
  apiBaseURL: string;
  fetchImpl?: typeof fetch;
}) {
  const noStoreFetch: typeof fetch = (input, init) =>
    fetchImpl(input, { ...init, cache: "no-store" });

  try {
    const { appointments } = await createBookingApi(
      apiBaseURL,
      noStoreFetch,
    ).getAppointments(status);
    return (
      <section className={styles.content}>
        <header className={styles.header}>
          <div>
            <p>Service schedule</p>
            <h1>Appointments</h1>
            <span>Browse every booked service appointment.</span>
          </div>
          <Link className={styles.bookButton} href="/">
            Book an appointment
          </Link>
        </header>
        <nav
          className={styles.filters}
          aria-label="Filter appointments by status"
        >
          {filters.map((filter) => {
            const active = filter.status === status;
            const href = filter.status
              ? `/appointments?status=${filter.status}`
              : "/appointments";
            return (
              <Link
                href={href}
                key={filter.label}
                aria-current={active ? "page" : undefined}
              >
                {filter.label}
              </Link>
            );
          })}
        </nav>
        {appointments.length ? (
          <div className={styles.list}>
            {appointments.map((appointment) => (
              <AppointmentCard key={appointment.id} appointment={appointment} />
            ))}
          </div>
        ) : (
          <div className={styles.empty}>
            <h2>No appointments found</h2>
            <p>Try another status filter or create a new booking.</p>
          </div>
        )}
      </section>
    );
  } catch (reason) {
    if (reason instanceof BookingApiError) {
      return (
        <section className={styles.empty}>
          <h1>Appointments unavailable</h1>
          <p>We could not retrieve appointments right now. Please try again.</p>
        </section>
      );
    }
    throw reason;
  }
}
