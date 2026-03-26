import torch
import numpy as np
from PIL import Image
import cv2
from ultralytics import YOLO
from typing import Tuple, List, Optional
import asyncio
from dataclasses import dataclass
import io
import time
from concurrent.futures import ThreadPoolExecutor

@dataclass
class VisionResult:
    is_valid: bool
    confidence: float
    reason: Optional[str]
    detected_objects: List[dict]
    analysis: dict
    inference_time_ms: float

class VisionService:
    def __init__(self, model_path: str, device: str, confidence_threshold: float = 0.6):
        self.model_path = model_path
        self.device = device
        self.confidence_threshold = confidence_threshold
        self.model = None
        self.is_loaded = False
        self.executor = ThreadPoolExecutor(max_workers=4)
        self.metrics = {
            "total_verifications": 0,
            "total_inference_time": 0,
            "successful_verifications": 0
        }
        
        # Infrastructure object classes (custom trained)
        self.infrastructure_classes = {
            0: "pothole",
            1: "burst_pipe",
            2: "flooding",
            3: "illegal_dumping",
            4: "broken_manhole",
            5: "cracked_sidewalk"
        }
    
    async def load_model(self):
        """Load YOLO model asynchronously"""
        loop = asyncio.get_event_loop()
        self.model = await loop.run_in_executor(
            self.executor,
            lambda: YOLO(self.model_path)
        )
        self.is_loaded = True
        
        # Warm up model
        dummy_input = np.zeros((640, 640, 3), dtype=np.uint8)
        await self._predict(dummy_input)
    
    async def verify_image(
        self,
        image_data: bytes,
        category: Optional[str] = None
    ) -> VisionResult:
        """Verify if image contains valid infrastructure issue"""
        if not self.is_loaded:
            raise RuntimeError("Model not loaded")
        
        start_time = time.time()
        
        # Convert bytes to image
        image = Image.open(io.BytesIO(image_data))
        image_np = np.array(image)
        
        # Run inference
        detections = await self._predict(image_np)
        
        inference_time = (time.time() - start_time) * 1000
        
        # Update metrics
        self.metrics["total_verifications"] += 1
        self.metrics["total_inference_time"] += inference_time
        
        # Process detections
        detected_objects = []
        max_confidence = 0.0
        best_match = None
        
        for det in detections:
            class_id = int(det[5])
            confidence = float(det[4])
            
            if class_id in self.infrastructure_classes:
                class_name = self.infrastructure_classes[class_id]
                detected_objects.append({
                    "class": class_name,
                    "confidence": confidence,
                    "bbox": det[:4].tolist()
                })
                
                if confidence > max_confidence:
                    max_confidence = confidence
                    best_match = class_name
        
        # Determine if valid based on confidence threshold
        is_valid = max_confidence >= self.confidence_threshold
        
        # If category specified, check if it matches
        if is_valid and category and best_match:
            is_valid = (best_match == category or 
                       self._is_related_category(best_match, category))
        
        # Generate analysis
        analysis = {
            "primary_issue": best_match if best_match else "none",
            "detection_count": len(detected_objects),
            "image_quality": self._assess_image_quality(image_np)
        }
        
        reason = None
        if not is_valid:
            if max_confidence < self.confidence_threshold:
                reason = f"No infrastructure issue detected with sufficient confidence (max: {max_confidence:.2f})"
            elif category and best_match != category:
                reason = f"Detected {best_match} but expected {category}"
        
        return VisionResult(
            is_valid=is_valid,
            confidence=max_confidence,
            reason=reason,
            detected_objects=detected_objects,
            analysis=analysis,
            inference_time_ms=inference_time
        )
    
    async def _predict(self, image: np.ndarray) -> List:
        """Run YOLO prediction"""
        loop = asyncio.get_event_loop()
        
        results = await loop.run_in_executor(
            self.executor,
            lambda: self.model(image, conf=self.confidence_threshold, verbose=False)
        )
        
        if len(results) > 0 and results[0].boxes is not None:
            return results[0].boxes.data.cpu().numpy()
        return []
    
    def _assess_image_quality(self, image: np.ndarray) -> dict:
        """Assess image quality metrics"""
        # Calculate blurriness using Laplacian variance
        gray = cv2.cvtColor(image, cv2.COLOR_RGB2GRAY)
        laplacian_var = cv2.Laplacian(gray, cv2.CV_64F).var()
        
        # Calculate brightness
        brightness = np.mean(gray)
        
        quality = "good"
        if laplacian_var < 100:
            quality = "blurry"
        elif brightness < 50:
            quality = "dark"
        elif brightness > 200:
            quality = "overexposed"
        
        return {
            "quality": quality,
            "blur_score": float(laplacian_var),
            "brightness": float(brightness)
        }
    
    def _is_related_category(self, detected: str, expected: str) -> bool:
        """Check if detected category is related to expected"""
        related_pairs = {
            ("flooding", "burst_pipe"): True,
            ("burst_pipe", "flooding"): True,
            ("pothole", "cracked_sidewalk"): True,
            ("cracked_sidewalk", "pothole"): True
        }
        return related_pairs.get((detected, expected), False)
    
    async def get_total_verifications(self) -> int:
        return self.metrics["total_verifications"]
    
    async def get_avg_inference_time(self) -> float:
        if self.metrics["total_verifications"] == 0:
            return 0.0
        return self.metrics["total_inference_time"] / self.metrics["total_verifications"]
    
    async def get_model_accuracy(self) -> float:
        # Implement accuracy calculation based on feedback loop
        return 0.92  # Placeholder
    
    async def cleanup(self):
        """Cleanup resources"""
        self.executor.shutdown(wait=True)