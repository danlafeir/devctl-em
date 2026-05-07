# Releases

Install or upgrade:

```bash
curl -fsSL https://raw.githubusercontent.com/danlafeir/em/main/scripts/install.sh | bash
```

Or upgrade an existing install:

```
em update
```

---

## v1.0.1 — 2026-05-07

**Patch pagination issue when querying GH workflow executions**

The bug only triggers when GitHub returns a Link header for the next page — meaning the repo had more than 100 workflow runs in  
  the queried date range. Repos with ≤100 runs get their results in one request and never go through doGetURL, so they work fine. 
So this trims the url that comes from the Link.

---


## v1.0.0 — 2026-04-30

**Release first version of the tool!**

This is the first release of all the features that have been incubating and iterating on! 
This tool codifies and centralizes all the signals I look at as an EM.

---
