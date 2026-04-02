# RichCode DEX UI

A React-based user interface for monitoring pump tokens, styled to match the GMGN interface. This application fetches token data from your backend API and displays it in a three-column layout with real-time updates.

## Features

- **Real-time Updates**: Automatically refreshes token data every 10 seconds
- **Three-Column Layout**: 
  - New Creations
  - Completing 
  - Completed
- **Dark Theme**: Modern dark UI matching the GMGN style
- **Responsive Design**: Works on desktop and mobile devices
- **Error Handling**: Graceful error handling with retry functionality

## Prerequisites

- Node.js (version 14 or higher)
- npm or yarn

## Installation

1. **Clone or create the project directory:**
```bash
mkdir dexs-ui
cd dexs-ui
```

2. **Install dependencies:**
```bash
npm install
```

## Configuration

The application is configured to work with your backend API at `http://127.0.0.1:8083`.

### Development Mode
- Uses proxy configuration to route API calls
- Automatically handles CORS issues

### Production Mode
- Makes direct API calls to the configured server

## Running the Application

### Development Mode
```bash
npm start
```
This will start the development server at `http://localhost:3000`

### Production Build
```bash
npm run build
npm install -g serve
serve -s build
```

## API Integration

The application expects your API endpoint `/v1/market/index_pump` to return token data. Currently, it includes mock data generation for demonstration purposes.

To integrate with your actual API:

1. Update the API response parsing in `src/components/TokenList.js`
2. Modify the token data structure in `src/components/TokenCard.js` to match your API response

## File Structure

```
dexs-ui/
├── public/
│   └── index.html
├── src/
│   ├── components/
│   │   ├── Header.js
│   │   ├── TokenCard.js
│   │   ├── TokenCard.css
│   │   ├── TokenList.js
│   │   └── TokenList.css
│   ├── App.js
│   ├── App.css
│   ├── index.js
│   └── index.css
├── package.json
└── README.md
```

## Customization

### Styling
- Edit CSS files to match your brand colors
- Modify the dark theme in `src/index.css`

### Data Structure
- Update token card layout in `src/components/TokenCard.js`
- Modify API integration in `src/components/TokenList.js`

### Polling Interval
- Change the refresh interval in `src/components/TokenList.js` (currently 10 seconds)

## Troubleshooting

### CORS Issues
If you encounter CORS issues:
1. Ensure your backend includes proper CORS headers
2. Use the proxy configuration in development mode
3. Consider using a reverse proxy in production

### API Connection Issues
1. Verify your backend server is running on port 8083
2. Check firewall settings
3. Ensure the API endpoint returns valid JSON

### Build Issues
1. Clear node_modules and reinstall dependencies
2. Check for any missing dependencies
3. Ensure Node.js version compatibility

## License

This project is for internal use and development purposes. 