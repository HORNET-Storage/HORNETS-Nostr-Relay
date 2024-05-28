import { useState, useEffect, useCallback } from 'react';

interface RelaySettings {
  mode: string;
  kinds: string[];
  photos: string[];
  videos: string[];
  gitNestr: string[];
  isKindsActive: boolean;
  isPhotosActive: boolean;
  isVideosActive: boolean;
  isGitNestrActive: boolean;
}

// Helper function to get initial settings from local storage
const getInitialSettings = (): RelaySettings => {
  const savedSettings = localStorage.getItem('relaySettings');
  return savedSettings ? JSON.parse(savedSettings) : {
    mode: 'smart',  // Ensure this default is correct
    kinds: [],
    photos: [],
    videos: [],
    gitNestr: [],
    isKindsActive: true,
    isPhotosActive: true,
    isVideosActive: true,
    isGitNestrActive: true,
  };
};

const useRelaySettings = () => {
  const [relaySettings, setRelaySettings] = useState<RelaySettings>(getInitialSettings());
  console.log("Relay Settings:", relaySettings);

  // Update local storage whenever relaySettings change
  useEffect(() => {
    localStorage.setItem('relaySettings', JSON.stringify(relaySettings));
  }, [relaySettings]);

  useEffect(() => {
    console.log("Updated Relay Settings:", relaySettings);
  }, [relaySettings]);

  // Function to update settings in state
  const updateSettings = useCallback((category: keyof RelaySettings, value: string | string[] | boolean) => {
    setRelaySettings(prevSettings => ({
      ...prevSettings,
      [category]: value
    }));
  }, []);

  // Function to save settings to server
  const saveSettings = useCallback(async () => {
    console.log("Saving relay settings to server...");
    try {
      const response = await fetch('http://localhost:5000/relay-settings', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ relay_settings: relaySettings })
      });
      if (!response.ok) {
        throw new Error(`Network response was not ok (status: ${response.status})`);
      }
      console.log("Settings saved successfully!");
    } catch (error) {
      console.error('Error:', error);
    }
  }, [relaySettings]);

  return { relaySettings, updateSettings, saveSettings };
};

export default useRelaySettings;
