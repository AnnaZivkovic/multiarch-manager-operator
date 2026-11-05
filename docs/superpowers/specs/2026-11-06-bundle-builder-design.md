# Preserve bundle builder during generation

Approved approach: snapshot `bundle.Dockerfile` before SDK generation, then restore its builder stage unchanged while refreshing generated metadata, core labels, custom labels, and builder COPY instructions. Dockerfile is sole source of builder image and operator digest; remove script's hardcoded builder.

Alternative: `--overwrite=false` would also suppress metadata updates, so reject it. Reading IMG after generation cannot work because SDK already removed it.

Scope: Makefile bundle recipe, patch script, existing shell regression test. No image changes or new dependencies. Missing builder must fail clearly rather than silently generate an unusable bundle.
