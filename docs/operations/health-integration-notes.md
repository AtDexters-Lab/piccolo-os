# Health Check Integration for Update Rollback

## Overview

The `/api/v1/health` endpoint we just implemented can be leveraged for Flatcar's update rollback mechanism. This ensures that if an update breaks piccolod, the system can automatically revert.

## Integration Points

### 1. Systemd Health Check Service

Create a companion service that periodically checks piccolod health:

```ini
# /etc/systemd/system/piccolod-health-check.service
[Unit]
Description=Piccolo Health Check
After=piccolod.service
Requires=piccolod.service

[Service]
Type=oneshot
ExecStart=/usr/bin/curl -f http://localhost:8080/api/v1/health
RemainAfterExit=yes
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

### 2. Update Engine Integration

Flatcar's `update_engine` can be configured to validate services after updates:

```ini
# /etc/systemd/system/update-engine.service.d/10-health-check.conf  
[Unit]
After=piccolod-health-check.service
Requires=piccolod-health-check.service
```

### 3. Health Check Criteria

The ecosystem endpoint returns three states:
- **healthy**: All systems operational → Continue with update
- **degraded**: Minor issues → Log warnings, continue
- **unhealthy**: Critical failures → Trigger rollback

## Implementation Benefits

✅ **Automated validation** - No manual intervention required  
✅ **Real-time assessment** - Tests actual runtime capabilities  
✅ **Granular reporting** - Individual component status available  
✅ **Rollback prevention** - Catches issues before they persist  

## Future Enhancements

1. **Configurable thresholds** - Define what constitutes "unhealthy"
2. **Historical tracking** - Monitor health trends over time  
3. **Alert integration** - Notify administrators of degraded states
4. **Recovery suggestions** - Provide actionable remediation steps

## Testing

Use the ecosystem endpoint to simulate various failure modes and validate rollback behavior:

```bash
# Test healthy state
curl http://localhost:8080/api/v1/health | jq '.overall'

# Simulate failure by stopping docker
systemctl stop docker
curl http://localhost:8080/api/v1/health | jq '.overall'
```