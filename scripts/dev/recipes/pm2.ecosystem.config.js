// pm2 ecosystem for teams already running pm2. pm2 is NOT a dependency of dws —
// this is one optional host, not the default. It supervises the FOREGROUND
// connector (not --daemon) to avoid double supervision. pm2 restarts on process
// death; pair with connect-watchdog (a pm2 cron_restart or an OS cron) for the
// alive-but-deaf case, which only `dws dev connect status --json` can detect.
//
//   pm2 start pm2.ecosystem.config.js
//   pm2 save && pm2 startup   // boot persistence
module.exports = {
  apps: [
    {
      name: 'dws-connect',
      script: 'dws',
      args: 'dev connect --robot-client-id REPLACE_CLIENT_ID --channel opencode',
      autorestart: true,
      restart_delay: 5000,
      max_restarts: 50,
      // Optional: periodic health-driven restart. A cleaner setup runs
      // connect-watchdog.sh from cron so restarts key off `status --json`.
      // cron_restart: '*/30 * * * *',
    },
  ],
}
