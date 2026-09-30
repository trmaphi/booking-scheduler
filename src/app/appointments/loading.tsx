import styles from "./appointments.module.css";

export default function AppointmentsLoading() {
  return (
    <section className={styles.content} role="status" aria-live="polite">
      <header className={styles.header}>
        <div>
          <p>Service schedule</p>
          <h1>Appointments</h1>
          <span>Loading appointments…</span>
        </div>
      </header>
      <div className={styles.loadingList} aria-hidden="true">
        <div />
        <div />
      </div>
    </section>
  );
}
