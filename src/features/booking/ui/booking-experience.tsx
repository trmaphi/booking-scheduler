"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";

import {
  BookingApiError,
  type AvailabilitySlot,
  type BookingApi,
  type BookingOptions,
} from "../api/contracts";
import { createBookingApi } from "../api/booking-api";
import styles from "./booking-experience.module.css";

type Step = "details" | "times" | "review";

interface Selection {
  vehicleId: string;
  dealershipId: string;
  serviceTypeId: string;
  date: string;
}

const emptySelection: Selection = {
  vehicleId: "",
  dealershipId: "",
  serviceTypeId: "",
  date: "",
};

function validTimeZone(value: string | undefined) {
  if (!value) return "UTC";
  try {
    new Intl.DateTimeFormat("en-GB", { timeZone: value }).format();
    return value;
  } catch {
    return "UTC";
  }
}

function formatTime(value: string, timeZone: string | undefined) {
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone: validTimeZone(timeZone),
  }).format(new Date(value));
}

function formatLongDate(value: string, timeZone: string | undefined) {
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: validTimeZone(timeZone),
  }).format(new Date(value));
}

function formatCalendarDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return "Selected date";
  const [, year, month, day] = match;
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(Number(year), Number(month) - 1, Number(day))));
}

function StepMarker({ current }: { current: Step }) {
  const activeIndex = current === "details" ? 0 : current === "times" ? 1 : 2;
  return (
    <ol className={styles.steps} aria-label="Booking progress">
      {["Details", "Time", "Confirm"].map((label, index) => (
        <li
          className={index <= activeIndex ? styles.stepActive : undefined}
          key={label}
        >
          <span>{index + 1}</span>
          {label}
        </li>
      ))}
    </ol>
  );
}

export function BookingExperience({ api }: { api?: BookingApi }) {
  const router = useRouter();
  const bookingApi = useMemo(() => {
    if (api) return api;
    const baseURL = process.env.NEXT_PUBLIC_API_BASE_URL;
    return baseURL ? createBookingApi(baseURL) : undefined;
  }, [api]);
  const [options, setOptions] = useState<BookingOptions>();
  const [selection, setSelection] = useState(emptySelection);
  const [slots, setSlots] = useState<AvailabilitySlot[]>([]);
  const [selectedStart, setSelectedStart] = useState("");
  const [step, setStep] = useState<Step>("details");
  const [loading, setLoading] = useState(Boolean(bookingApi));
  const [error, setError] = useState(
    bookingApi
      ? ""
      : "Booking is not configured. Set NEXT_PUBLIC_API_BASE_URL to the local Go API URL.",
  );

  useEffect(() => {
    if (!bookingApi) {
      return;
    }
    let active = true;
    bookingApi
      .getBookingOptions()
      .then((result) => {
        if (active) setOptions(result);
      })
      .catch(() => {
        if (active)
          setError("We could not load booking details. Please try again.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [bookingApi]);

  const selected = useMemo(
    () => ({
      vehicle: options?.vehicles.find(({ id }) => id === selection.vehicleId),
      dealership: options?.dealerships.find(
        ({ id }) => id === selection.dealershipId,
      ),
      serviceType: options?.serviceTypes.find(
        ({ id }) => id === selection.serviceTypeId,
      ),
      slot: slots.find(({ startAt }) => startAt === selectedStart),
    }),
    [options, selectedStart, selection, slots],
  );

  const selectionComplete = Object.values(selection).every(Boolean);

  async function loadAvailability() {
    if (!selectionComplete || !bookingApi) return;
    setLoading(true);
    setError("");
    setSelectedStart("");
    try {
      const result = await bookingApi.getAvailability(selection);
      setSlots(result.slots);
      setStep("times");
    } catch {
      setError("Availability could not be loaded. Please try again.");
    } finally {
      setLoading(false);
    }
  }

  async function confirmAppointment() {
    if (!selectedStart || !bookingApi) return;
    setLoading(true);
    setError("");
    try {
      const confirmed = await bookingApi.confirmAppointment({
        ...selection,
        startAt: selectedStart,
        idempotencyKey: crypto.randomUUID(),
      });
      router.push(`/appointments/${confirmed.id}`);
    } catch (reason) {
      if (
        reason instanceof BookingApiError &&
        reason.code === "RESOURCE_CONFLICT"
      ) {
        const staleStart = selectedStart;
        setSlots([]);
        setSelectedStart("");
        setStep("times");
        try {
          const refreshed = await bookingApi.getAvailability(selection);
          setSlots(
            refreshed.slots.filter(({ startAt }) => startAt !== staleStart),
          );
          setError(
            "That time was just taken. We refreshed the available times for you.",
          );
        } catch {
          setError(
            "That time is no longer available, and we could not refresh availability. Please try again.",
          );
        }
      } else {
        setError("We could not confirm this appointment. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  }

  function updateSelection(field: keyof Selection, value: string) {
    setSelection((current) => ({ ...current, [field]: value }));
    if (step !== "details") {
      setStep("details");
      setSlots([]);
      setSelectedStart("");
    }
    setError("");
  }

  if (loading && !options) {
    return (
      <div className={styles.loading} role="status">
        <span />
        Preparing your booking…
      </div>
    );
  }

  if (!options) {
    return (
      <div className={styles.error} role="alert">
        {error}
      </div>
    );
  }

  return (
    <section className={styles.bookingCard}>
      <StepMarker current={step} />
      <div className={styles.cardHeader}>
        <p className={styles.eyebrow}>Service booking</p>
        <h2>
          {step === "review"
            ? "Review your appointment"
            : step === "times"
              ? "Choose another time"
              : "Tell us what you need"}
        </h2>
        <p>
          {step === "review"
            ? "Check the details before we reserve your resources."
            : "Choose your vehicle, preferred centre and service."}
        </p>
      </div>

      {error ? (
        <div className={styles.error} role="alert">
          {error}
        </div>
      ) : null}

      <div className={styles.formGrid} aria-label="Appointment details">
        <label>
          <span>Vehicle</span>
          <select
            value={selection.vehicleId}
            onChange={(event) =>
              updateSelection("vehicleId", event.target.value)
            }
          >
            <option value="">Select a vehicle</option>
            {options.vehicles.map((vehicle) => (
              <option value={vehicle.id} key={vehicle.id}>
                {vehicle.label} · {vehicle.registration}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Service centre</span>
          <select
            value={selection.dealershipId}
            onChange={(event) =>
              updateSelection("dealershipId", event.target.value)
            }
          >
            <option value="">Select a centre</option>
            {options.dealerships.map((dealership) => (
              <option value={dealership.id} key={dealership.id}>
                {dealership.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Service</span>
          <select
            value={selection.serviceTypeId}
            onChange={(event) =>
              updateSelection("serviceTypeId", event.target.value)
            }
          >
            <option value="">Select a service</option>
            {options.serviceTypes.map((service) => (
              <option value={service.id} key={service.id}>
                {service.name} · {service.durationMinutes} min
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Preferred date</span>
          <input
            type="date"
            min="2026-09-29"
            value={selection.date}
            onChange={(event) => updateSelection("date", event.target.value)}
          />
        </label>
      </div>

      {step === "details" ? (
        <div className={styles.actions}>
          <span className={styles.hint}>
            No payment is needed to reserve a time.
          </span>
          <button
            className={styles.primaryButton}
            disabled={!selectionComplete || loading}
            onClick={loadAvailability}
          >
            Find available times
          </button>
        </div>
      ) : null}

      {step === "times" ? (
        <div className={styles.timesSection}>
          <div className={styles.sectionTitle}>
            <div>
              <span className={styles.eyebrow}>Available on</span>
              <h3>
                {selection.date
                  ? formatCalendarDate(selection.date)
                  : "Selected date"}
              </h3>
            </div>
            <span className={styles.duration}>
              {selected.serviceType?.durationMinutes ?? 0} min
            </span>
          </div>
          {loading ? (
            <div className={styles.loading} role="status">
              <span />
              Checking live availability…
            </div>
          ) : slots.length === 0 ? (
            <div className={styles.emptyState}>
              <strong>No times are available</strong>
              <p>Try another date or service centre.</p>
            </div>
          ) : (
            <fieldset className={styles.timeGrid}>
              <legend className="srOnly">Available appointment times</legend>
              {slots.map((slot) => (
                <label
                  key={slot.startAt}
                  className={
                    selectedStart === slot.startAt
                      ? styles.timeSelected
                      : undefined
                  }
                >
                  <input
                    type="radio"
                    name="appointment-time"
                    value={slot.startAt}
                    checked={selectedStart === slot.startAt}
                    onChange={() => setSelectedStart(slot.startAt)}
                  />
                  <strong>
                    {formatTime(slot.startAt, selected.dealership?.timezone)}
                  </strong>
                  <small>
                    to {formatTime(slot.endAt, selected.dealership?.timezone)}
                  </small>
                </label>
              ))}
            </fieldset>
          )}
          <div className={styles.actions}>
            <button
              className={styles.textButton}
              onClick={() => setStep("details")}
            >
              Back
            </button>
            <button
              className={styles.primaryButton}
              disabled={!selectedStart}
              onClick={() => setStep("review")}
            >
              Review appointment
            </button>
          </div>
        </div>
      ) : null}

      {step === "review" &&
      selected.vehicle &&
      selected.dealership &&
      selected.serviceType &&
      selected.slot ? (
        <div className={styles.review}>
          <div className={styles.reviewRows}>
            <div>
              <span>Vehicle</span>
              <strong>{selected.vehicle.label}</strong>
              <small>{selected.vehicle.registration}</small>
            </div>
            <div>
              <span>Service centre</span>
              <strong>{selected.dealership.name}</strong>
              <small>{selected.dealership.address}</small>
            </div>
            <div>
              <span>Service</span>
              <strong>
                {selected.serviceType.name} ·{" "}
                {selected.serviceType.durationMinutes} minutes
              </strong>
              <small>{selected.serviceType.description}</small>
            </div>
            <div>
              <span>Date and time</span>
              <strong>
                {formatLongDate(
                  selected.slot.startAt,
                  selected.dealership.timezone,
                )}
              </strong>
              <small>
                {formatTime(
                  selected.slot.startAt,
                  selected.dealership.timezone,
                )}
                –{formatTime(selected.slot.endAt, selected.dealership.timezone)}
              </small>
            </div>
          </div>
          <div className={styles.assignmentNote}>
            <span aria-hidden="true">i</span> A qualified technician and service
            bay are assigned when you confirm.
          </div>
          <div className={styles.actions}>
            <button
              className={styles.textButton}
              onClick={() => setStep("times")}
            >
              Back to times
            </button>
            <button
              className={styles.primaryButton}
              disabled={loading}
              onClick={confirmAppointment}
            >
              {loading ? "Confirming…" : "Confirm appointment"}
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
