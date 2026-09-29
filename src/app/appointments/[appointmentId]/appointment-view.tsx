import Link from "next/link";

import { createBookingApi } from "@/features/booking/api/booking-api";
import {
  BookingApiError,
  type ConfirmedAppointment,
} from "@/features/booking/api/contracts";
import styles from "@/features/booking/ui/booking-experience.module.css";

function formatDate(value: string, timeZone: string) {
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
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

export async function loadAppointment(
  appointmentId: string,
  apiBaseURL: string,
  fetchImpl: typeof fetch = globalThis.fetch,
) {
  const noStoreFetch: typeof fetch = (input, init) =>
    fetchImpl(input, { ...init, cache: "no-store" });
  return createBookingApi(apiBaseURL, noStoreFetch).getAppointment(
    appointmentId,
  );
}

function ConfirmedView({ appointment }: { appointment: ConfirmedAppointment }) {
  const { timezone } = appointment.dealership;
  return (
    <section className={styles.confirmed}>
      <div className={styles.successMark} aria-hidden="true">
        ✓
      </div>
      <p className={styles.eyebrow}>Booking reference {appointment.id}</p>
      <h1>Appointment confirmed</h1>
      <p className={styles.confirmedLead}>
        <strong>{appointment.status}</strong> · Your service resources are
        reserved.
      </p>
      <div className={styles.confirmedGrid}>
        <div>
          <span>Vehicle</span>
          <strong>{appointment.vehicle.label}</strong>
          <small>{appointment.vehicle.registration}</small>
        </div>
        <div>
          <span>Service</span>
          <strong>{appointment.serviceType.name}</strong>
          <small>{appointment.serviceType.durationMinutes} minutes</small>
        </div>
        <div>
          <span>Date and time</span>
          <strong>{formatDate(appointment.startAt, timezone)}</strong>
          <small>
            {formatTime(appointment.startAt, timezone)}–
            {formatTime(appointment.endAt, timezone)}
          </small>
        </div>
        <div>
          <span>Service centre</span>
          <strong>{appointment.dealership.name}</strong>
          <small>{appointment.dealership.address}</small>
        </div>
        <div>
          <span>Technician</span>
          <strong>{appointment.technician.name}</strong>
          <small>Qualified for this service</small>
        </div>
        <div>
          <span>Service bay</span>
          <strong>{appointment.serviceBay.name}</strong>
          <small>Reserved for the full appointment</small>
        </div>
      </div>
      <Link className={styles.secondaryButton} href="/">
        Book another service
      </Link>
    </section>
  );
}

function StatusView({ title, message }: { title: string; message: string }) {
  return (
    <section className={styles.bookingCard}>
      <p className={styles.eyebrow}>Service booking</p>
      <h1>{title}</h1>
      <p>{message}</p>
      <Link className={styles.secondaryButton} href="/">
        Return to booking
      </Link>
    </section>
  );
}

export async function renderAppointmentPage({
  appointmentId,
  apiBaseURL,
  fetchImpl = globalThis.fetch,
}: {
  appointmentId: string;
  apiBaseURL: string;
  fetchImpl?: typeof fetch;
}) {
  try {
    const appointment = await loadAppointment(
      appointmentId,
      apiBaseURL,
      fetchImpl,
    );
    return <ConfirmedView appointment={appointment} />;
  } catch (reason) {
    if (reason instanceof BookingApiError && reason.code === "NOT_FOUND") {
      return (
        <StatusView
          title="Appointment not found"
          message="Check the booking link or return to start a new booking."
        />
      );
    }
    if (reason instanceof BookingApiError) {
      return (
        <StatusView
          title="Appointment unavailable"
          message="We could not retrieve this appointment right now. Please try again."
        />
      );
    }
    throw reason;
  }
}
