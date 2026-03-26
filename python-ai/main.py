import asyncio
import json
import logging
import time
from contextlib import asynccontextmanager
from typing import Optional
import os
from datetime import datetime

import httpx
import redis.asyncio as redis
from fastapi import FastAPI, HTTPException, BackgroundTasks, File, UploadFile, Form
from fastapi.middleware.cors import CORSMiddleware
from fastapi.middleware.trustedhost import TrustedHostMiddleware
from fastapi.responses import JSONResponse
from prometheus_fastapi_instrumentator import Instrumentator
from pydantic import BaseModel, Field
import boto3
from botocore.config import Config
from tenacity import retry, stop_after_attempt, wait_exponential

from services.vision_service import VisionService
from services.llm_service import LLMService
from services.cache_service import CacheService
from models.schemas import VerifyRequest, VerifyResponse, HealthResponse
from utils.logger import setup_logger

# Configure logging
logger = setup_logger(__name__)

# Global service instances
vision_service = None
llm_service = None
cache_service = None
redis_client = None
s3_client = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup/shutdown events"""
    global vision_service, llm_service, cache_service, redis_client, s3_client
    
    # Startup
    logger.info("Starting AI Verification Service...")
    
    # Initialize Redis for caching
    redis_client = await redis.from_url(
        os.getenv("REDIS_URL", "redis://localhost:6379/0"),
        encoding="utf-8",
        decode_responses=True
    )
    
    # Initialize services
    vision_service = VisionService(
        model_path=os.getenv("MODEL_PATH", "models/infrastructure_model.pt"),
        device=os.getenv("DEVICE", "cuda" if torch.cuda.is_available() else "cpu"),
        confidence_threshold=float(os.getenv("CONFIDENCE_THRESHOLD", "0.6"))
    )
    
    llm_service = LLMService(
        api_key=os.getenv("OPENAI_API_KEY"),
        model=os.getenv("LLM_MODEL", "gpt-3.5-turbo"),
        use_cache=os.getenv("USE_LLM_CACHE", "true").lower() == "true"
    )
    
    cache_service = CacheService(redis_client, ttl=3600)  # 1 hour cache
    
    # Initialize S3 client for image processing
    s3_config = Config(
        region_name=os.getenv("AWS_REGION", "us-east-1"),
        signature_version="s3v4"
    )
    s3_client = boto3.client(
        "s3",
        aws_access_key_id=os.getenv("AWS_ACCESS_KEY_ID"),
        aws_secret_access_key=os.getenv("AWS_SECRET_ACCESS_KEY"),
        config=s3_config
    )
    
    # Load model and warm up
    await vision_service.load_model()
    logger.info("Vision model loaded successfully")
    
    yield
    
    # Shutdown
    logger.info("Shutting down AI Verification Service...")
    await redis_client.close()
    await vision_service.cleanup()

app = FastAPI(
    title="Mji-Smart AI Verification Service",
    version="1.0.0",
    lifespan=lifespan,
    docs_url="/docs" if os.getenv("ENVIRONMENT") != "production" else None
)

# Middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)
app.add_middleware(TrustedHostMiddleware, allowed_hosts=["*"])

# Prometheus metrics
instrumentator = Instrumentator().instrument(app).expose(app)

# Models
class BatchVerifyRequest(BaseModel):
    reports: list[VerifyRequest]

class ModelMetrics(BaseModel):
    total_verifications: int
    avg_inference_time: float
    cache_hit_rate: float
    model_accuracy: float
    last_updated: datetime

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy",
        timestamp=datetime.utcnow(),
        model_loaded=vision_service.is_loaded,
        cache_available=await cache_service.is_available(),
        version="1.0.0"
    )

@app.post("/verify", response_model=VerifyResponse)
@retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=4, max=10))
async def verify_single_report(
    request: VerifyRequest,
    background_tasks: BackgroundTasks
):
    """
    Verify a single infrastructure report with AI
    """
    start_time = time.time()
    
    try:
        # Check cache first
        cache_key = f"verification:{request.report_id}"
        cached_result = await cache_service.get(cache_key)
        
        if cached_result:
            logger.info(f"Cache hit for report {request.report_id}")
            response = VerifyResponse(**cached_result)
            response.from_cache = True
            return response
        
        # Download image from URL
        image_data = await download_image(request.image_url)
        
        # Step 1: Computer Vision verification
        vision_result = await vision_service.verify_image(
            image_data,
            category=request.category
        )
        
        if not vision_result.is_valid:
            return VerifyResponse(
                report_id=request.report_id,
                verified=False,
                severity=0,
                ai_confidence=vision_result.confidence,
                reason=vision_result.reason,
                process_time_ms=int((time.time() - start_time) * 1000)
            )
        
        # Step 2: LLM severity scoring
        severity = await llm_service.score_severity(
            description=request.description,
            category=request.category,
            image_analysis=vision_result.analysis
        )
        
        # Step 3: Combine results
        final_confidence = (vision_result.confidence * 0.7 + severity.confidence * 0.3)
        
        response = VerifyResponse(
            report_id=request.report_id,
            verified=True,
            severity=severity.score,
            ai_confidence=final_confidence,
            vision_confidence=vision_result.confidence,
            llm_confidence=severity.confidence,
            detected_objects=vision_result.detected_objects,
            severity_reason=severity.reason,
            process_time_ms=int((time.time() - start_time) * 1000)
        )
        
        # Cache result for 24 hours
        background_tasks.add_task(
            cache_service.set,
            cache_key,
            response.dict(),
            ttl=86400
        )
        
        # Send metrics
        background_tasks.add_task(
            send_verification_metrics,
            response.dict()
        )
        
        return response
        
    except Exception as e:
        logger.error(f"Verification failed for report {request.report_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/verify/batch")
async def verify_batch_reports(
    request: BatchVerifyRequest,
    background_tasks: BackgroundTasks
):
    """
    Batch verify multiple reports (for high throughput)
    """
    tasks = []
    for report in request.reports:
        tasks.append(verify_single_report(report, background_tasks))
    
    results = await asyncio.gather(*tasks, return_exceptions=True)
    
    return {
        "total": len(results),
        "successful": sum(1 for r in results if isinstance(r, VerifyResponse)),
        "failed": sum(1 for r in results if isinstance(r, Exception)),
        "results": results
    }

@app.get("/model/metrics")
async def get_model_metrics():
    """Get model performance metrics"""
    return ModelMetrics(
        total_verifications=await vision_service.get_total_verifications(),
        avg_inference_time=await vision_service.get_avg_inference_time(),
        cache_hit_rate=await cache_service.get_hit_rate(),
        model_accuracy=await vision_service.get_model_accuracy(),
        last_updated=datetime.utcnow()
    )

async def download_image(url: str) -> bytes:
    """Download image from URL with timeout and retry"""
    async with httpx.AsyncClient(timeout=10.0) as client:
        for attempt in range(3):
            try:
                response = await client.get(url)
                response.raise_for_status()
                return response.content
            except httpx.HTTPError as e:
                if attempt == 2:
                    raise
                await asyncio.sleep(2 ** attempt)

async def send_verification_metrics(verification_data: dict):
    """Send metrics to monitoring system"""
    # Implement your metrics reporting here
    # Could be Prometheus, DataDog, etc.
    pass

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=8000,
        reload=os.getenv("ENVIRONMENT") == "development",
        workers=int(os.getenv("WORKERS", 4))
    )