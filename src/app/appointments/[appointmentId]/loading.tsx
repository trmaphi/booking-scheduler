import styles from "@/features/booking/ui/booking-experience.module.css";

export default function AppointmentLoading() {
  return (
    <main className="pageShell">
      <div className={styles.loading} role="status">
        <span />
        Loading your appointment…
      </div>
    </main>
  );
}
