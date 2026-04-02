import axios from 'axios';

// Shared Axios instance
// CRA dev uses proxy (package.json "proxy"), so keep baseURL empty.
// In production, requests should use absolute paths configured by the server/proxy.
const api = axios.create({});

export default api;

