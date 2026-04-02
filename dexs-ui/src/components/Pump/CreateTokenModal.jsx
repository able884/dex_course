import React, { useState, useEffect } from 'react';
import { useWallet, useConnection } from '@solana/wallet-adapter-react';
import { Transaction, Keypair } from '@solana/web3.js';
import { Buffer } from 'buffer';

import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '../UI/dialog';
import { Button } from '../UI/Button';
import { Input } from '../UI/input';
import { LoadingSpinner } from '../UI/loading-spinner';

const API_URL = process.env.NODE_ENV === 'development' ? '' : '/api';

const CreateTokenModal = ({ isOpen, onClose }) => {
  const { publicKey, connected, signTransaction, sendTransaction } = useWallet();
  const { connection } = useConnection();

  const [name, setName] = useState('');
  const [symbol, setSymbol] = useState('');
  const [uri, setUri] = useState('');
  const [description, setDescription] = useState('');
  const [imageFile, setImageFile] = useState(null);
  const [website, setWebsite] = useState('');
  const [twitter, setTwitter] = useState('');
  const [telegram, setTelegram] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [txSignature, setTxSignature] = useState('');
  const [copyHint, setCopyHint] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    setName('');
    setSymbol('');
    setUri('');
    setDescription('');
    setImageFile(null);
    setWebsite('');
    setTwitter('');
    setTelegram('');
    setIsLoading(false);
    setError('');
    setSuccess('');
    setTxSignature('');
    setCopyHint('');
  }, [isOpen]);

  const reportCreateResult = async ({ chain_id, mint, user_wallet_address, tx_signature, success, error_message }) => {
    try {
      await fetch(`${API_URL}/v1/pump/report_create_result`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chain_id, mint, user_wallet_address, tx_signature, success, error_message }),
      });
    } catch (err) {
      console.warn('report_create_result failed:', err);
    }
  };

  const compressImage = async (file, maxWidth = 1024, quality = 0.8) => {
    try {
      const img = document.createElement('img');
      const reader = new FileReader();
      const dataUrl = await new Promise((resolve, reject) => {
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsDataURL(file);
      });
      await new Promise((resolve, reject) => {
        img.onload = resolve;
        img.onerror = reject;
        img.src = dataUrl;
      });
      const scale = Math.min(1, maxWidth / img.width);
      const canvas = document.createElement('canvas');
      canvas.width = Math.floor(img.width * scale);
      canvas.height = Math.floor(img.height * scale);
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
      const blob = await new Promise((resolve) => canvas.toBlob(resolve, 'image/jpeg', quality));
      return new File([blob], file.name.replace(/\.[^.]+$/, '.jpg'), { type: 'image/jpeg' });
    } catch (e) {
      return file;
    }
  };

  const uploadMetadata = async (meta, file) => {
    let image_b64 = '';
    if (file) {
      const inputFile = await compressImage(file);
      image_b64 = await new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve((reader.result || '').toString().split(',')[1] || '');
        reader.onerror = reject;
        reader.readAsDataURL(inputFile);
      });
    }
    const payload = { ...meta };
    if (!payload.image_uri && image_b64) payload.image_content_base64 = image_b64;
    const resp = await fetch(`${API_URL}/v1/ipfs/upload_metadata`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.message || 'IPFS metadata upload failed');
    }
    return data.data?.uri || data.uri || data.url;
  };

  const uploadMetadataSimple = async (meta) => {
    const resp = await fetch(`${API_URL}/v1/ipfs/upload_metadata`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(meta),
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.message || 'IPFS metadata upload failed');
    }
    return data.data?.uri || data.uri || data.url;
  };

  const handleCreate = async () => {
    setError('');
    setSuccess('');
    setTxSignature('');
    setCopyHint('');

    if (!connected || !publicKey) {
      setError('Please connect your wallet');
      return;
    }
    if (!signTransaction || !sendTransaction) {
      setError('Wallet does not support signing');
      return;
    }
    if (!name || !symbol) {
      setError('Please fill in name and symbol');
      return;
    }

    let mintKeypair;
    try {
      setIsLoading(true);
      setSuccess('Preparing unsigned transaction...');

      let finalUri = uri;
      let imageContentBase64 = '';
      if (!finalUri && imageFile) {
        const inputFile = await compressImage(imageFile);
        imageContentBase64 = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve((reader.result || '').toString().split(',')[1] || '');
          reader.onerror = reject;
          reader.readAsDataURL(inputFile);
        });
      }

      setSuccess('Building unsigned transaction...');
      mintKeypair = Keypair.generate();

      const payload = {
        chain_id: 100000,
        name,
        symbol,
        uri: finalUri,
        website,
        twitter,
        telegram,
        description,
        image_uri: '',
        image_content_base64: imageContentBase64,
        mint: mintKeypair.publicKey.toBase58(),
        user_wallet_address: publicKey.toString(),
      };

      const resp = await fetch(`${API_URL}/v1/pump/create_token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      const data = await resp.json();
      if (!resp.ok) {
        setError(data.message || 'Server error');
        return;
      }

      const txBase64 = data.data?.tx || data.tx || data.tx_hash || data.txHash;
      if (!txBase64) {
        setError('No unsigned transaction returned');
        return;
      }

      const raw = Buffer.from(txBase64, 'base64');
      const tx = Transaction.from(raw);
      tx.partialSign(mintKeypair);

      try {
        const sim = await connection.simulateTransaction(tx, { sigVerify: false, commitment: 'processed' });
        if (sim.value?.err) {
          const logs = sim.value?.logs?.join('\n') || JSON.stringify(sim.value.err);
          setError(`Simulation failed: ${logs}`);
          setIsLoading(false);
          return;
        }
      } catch (simErr) {
        console.warn('Simulation error (ignored):', simErr);
      }

      setSuccess('Submitting transaction...');
      let sig;
      try {
        if (signTransaction) {
          const signedTx = await signTransaction(tx);
          sig = await connection.sendRawTransaction(signedTx.serialize(), {
            skipPreflight: false,
            preflightCommitment: 'confirmed',
            maxRetries: 2,
          });
        } else {
          sig = await sendTransaction(tx, connection, { preflightCommitment: 'confirmed', maxRetries: 2 });
        }
      } catch (walletErr) {
        try {
          const rawSig = await connection.sendRawTransaction(tx.serialize());
          sig = rawSig;
        } catch (e2) {
          throw walletErr;
        }
      }

      setSuccess('Token submitted successfully');
      setTxSignature(sig);
      setCopyHint('');
      reportCreateResult({
        chain_id: 100000,
        mint: mintKeypair.publicKey.toBase58(),
        user_wallet_address: publicKey.toString(),
        tx_signature: sig,
        success: true,
        error_message: '',
      });
    } catch (e) {
      console.error(e);
      setError(e.message || 'Failed to create token');
      setTxSignature('');
      setCopyHint('');
      reportCreateResult({
        chain_id: 100000,
        mint: mintKeypair ? mintKeypair.publicKey.toBase58() : '',
        user_wallet_address: publicKey?.toString?.() || '',
        tx_signature: '',
        success: false,
        error_message: e.message || 'unknown error',
      });
    } finally {
      setIsLoading(false);
    }
  };

  const handleCopySignature = async () => {
    if (!txSignature) return;
    try {
      if (!navigator || !navigator.clipboard) {
        throw new Error('Clipboard unavailable');
      }
      await navigator.clipboard.writeText(txSignature);
      setCopyHint('Signature copied');
    } catch (err) {
      setCopyHint('Copy failed');
    } finally {
      setTimeout(() => setCopyHint(''), 2000);
    }
  };

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="w-full max-w-[520px]">
        <DialogHeader>
          <DialogTitle>Create PumpFun Token</DialogTitle>
        </DialogHeader>

        <div className="space-y-3">
          <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input placeholder="Symbol" value={symbol} onChange={(e) => setSymbol(e.target.value)} />
          <Input placeholder="Description (optional)" value={description} onChange={(e) => setDescription(e.target.value)} />
          <div className="text-sm text-muted-foreground">
            If URI is empty, the backend uploads metadata + image to IPFS automatically
          </div>
          <Input type="file" accept="image/*" onChange={(e) => setImageFile(e.target.files?.[0] || null)} />
          <Input placeholder="Website (optional)" value={website} onChange={(e) => setWebsite(e.target.value)} />
          <Input placeholder="Twitter (optional)" value={twitter} onChange={(e) => setTwitter(e.target.value)} />
          <Input placeholder="Telegram (optional)" value={telegram} onChange={(e) => setTelegram(e.target.value)} />
          <Input placeholder="Metadata URI (optional)" value={uri} onChange={(e) => setUri(e.target.value)} />
          {error && <div className="text-red-500 text-sm">{error}</div>}
          {success && (
            <div className="text-green-500 text-sm space-y-1">
              <div>{success}</div>
              {txSignature && (
                <div className="flex items-center space-x-2 text-xs">
                  <span className="text-muted-foreground">Signature:</span>
                  <button
                    type="button"
                    onClick={handleCopySignature}
                    className="font-mono text-blue-500 hover:text-blue-400 truncate max-w-[260px]"
                    title={txSignature}
                  >
                    {txSignature}
                  </button>
                </div>
              )}
              {copyHint && <div className="text-xs text-blue-400">{copyHint}</div>}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleCreate} disabled={isLoading}>
            {isLoading ? <LoadingSpinner className="mr-2" /> : null}
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default CreateTokenModal;
