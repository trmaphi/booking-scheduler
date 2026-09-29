import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import path from "node:path";

import { bookingFixture } from "./fixtures";

test("persists a confirmed appointment across reload", async ({
  page,
  request,
}) => {
  await page.goto("/");

  await page
    .getByRole("combobox", { name: /^Vehicle/ })
    .selectOption({ label: bookingFixture.vehicle });
  await page
    .getByRole("combobox", { name: /^Service centre/ })
    .selectOption({ label: bookingFixture.centre });
  await page
    .getByRole("combobox", { name: /^Service(?! centre)/ })
    .selectOption({ label: bookingFixture.service });
  await page.getByLabel("Preferred date").fill(bookingFixture.date);
  await page.getByRole("button", { name: "Find available times" }).click();

  const firstSlot = page.getByRole("radio").first();
  await expect(firstSlot).toBeVisible();
  await firstSlot.check();
  const interval = (await firstSlot.locator("xpath=..").innerText())
    .replace(/\s+/g, " ")
    .trim();
  const [startTime, endTime] = interval.split(" to ");
  await page.getByRole("button", { name: "Review appointment" }).click();

  await expect(page.getByText(bookingFixture.duration)).toBeVisible();
  await expect(
    page.getByText(new RegExp(`${startTime}.*${endTime}`)),
  ).toBeVisible();
  await expect(
    page.getByText(bookingFixture.technician, { exact: true }),
  ).toHaveCount(0);
  await expect(page.getByText(bookingFixture.bay, { exact: true })).toHaveCount(
    0,
  );

  await page.getByRole("button", { name: "Confirm appointment" }).click();
  await expect(page).toHaveURL(/\/appointments\/([0-9a-f-]{36})$/);
  const appointmentId = page.url().split("/").at(-1)!;

  await expect(
    page.getByRole("heading", { name: "Appointment confirmed" }),
  ).toBeVisible();
  await expect(page.getByText(appointmentId, { exact: false })).toBeVisible();
  await expect(page.getByText("CONFIRMED", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Silver Hatchback", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Routine Inspection", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(bookingFixture.centre, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(bookingFixture.technician, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(bookingFixture.bay, { exact: true }),
  ).toBeVisible();

  const persistedInterval = await page
    .getByText(/^\d{2}:\d{2}–\d{2}:\d{2}$/)
    .textContent();
  await page.reload();
  await expect(page.getByText(appointmentId, { exact: false })).toBeVisible();
  await expect(
    page.getByText(bookingFixture.technician, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(bookingFixture.bay, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText(persistedInterval!, { exact: true }),
  ).toBeVisible();

  const apiBaseURL = process.env.E2E_API_BASE_URL ?? "http://localhost:8080";
  const response = await request.get(
    `${apiBaseURL}/api/v1/appointments/${appointmentId}`,
  );
  expect(response.ok()).toBeTruthy();
  const appointment = await response.json();
  const expectedAppointment = {
    id: appointmentId,
    status: "CONFIRMED",
    vehicle: {
      id: appointment.vehicle.id,
      customerId: appointment.vehicle.customerId,
      label: bookingFixture.vehicleLabel,
      registration: bookingFixture.registration,
    },
    dealership: {
      id: appointment.dealership.id,
      name: bookingFixture.centre,
      address: bookingFixture.centreAddress,
      timezone: bookingFixture.centreTimezone,
    },
    serviceType: {
      id: appointment.serviceType.id,
      name: bookingFixture.serviceName,
      description: bookingFixture.serviceDescription,
      durationMinutes: bookingFixture.durationMinutes,
    },
    technician: {
      id: appointment.technician.id,
      name: bookingFixture.technician,
    },
    serviceBay: {
      id: appointment.serviceBay.id,
      name: bookingFixture.bay,
    },
    startAt: "2030-01-03T09:00:00Z",
    endAt: "2030-01-03T10:00:00Z",
  };
  expect(appointment).toEqual(expectedAppointment);

  const outputDir = process.env.E2E_OUTPUT_DIR ?? "./test-results";
  await writeFile(
    path.join(outputDir, "expected-appointment.json"),
    JSON.stringify(expectedAppointment),
  );
});
